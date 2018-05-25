package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	codecs    = serializer.NewCodecFactory(runtime.NewScheme())
	jsonPatch = v1beta1.PatchTypeJSONPatch
)

const (
	basicAuthSidecarImage = "docker.io/alvaroaleman/k8s-auth-injector-sidecar"
	basicAuthSidecarName  = "auth-sidecar"
	annotationSecretName  = "authinjector/basic-auth-secret-name"
	annotationPortName    = "authinjector/port-name"
)

type jsonPatchElement struct {
	OP    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func mutate(ar v1beta1.AdmissionReview) (*v1beta1.AdmissionResponse, error) {
	glog.V(4).Info("Processing request...")
	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if ar.Request.Resource != podResource {
		return nil, fmt.Errorf("expect resource to be %s", podResource)
	}

	raw := ar.Request.Object.Raw
	pod := corev1.Pod{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, &pod); err != nil {
		return nil, fmt.Errorf("failed to deserialize: %v", err)
	}

	reviewResponse := v1beta1.AdmissionResponse{}
	reviewResponse.Allowed = true

	if secretName, exists := pod.Annotations[annotationSecretName]; exists {
		upstreamPort, err := getUpstreamPortFromPod(pod)
		if err != nil {
			return nil, err
		}
		listenPortNumber := getUnusedContainerPort(pod)
		volumeMountName := fmt.Sprintf("authinjector-basic-auth-secret-%s", secretName)
		upstreamContainerIndex, upstreamPortIndex, err := getContainerAndPortIndexForNamedPort(pod, upstreamPort.Name)
		if err != nil {
			return nil, err
		}

		sidecar := corev1.Container{Name: basicAuthSidecarName,
			Image: basicAuthSidecarImage,
			Ports: []corev1.ContainerPort{*upstreamPort}}
		sidecar.Env = []corev1.EnvVar{corev1.EnvVar{Name: "UPSTREAM_PORT",
			Value: strconv.Itoa(int(upstreamPort.ContainerPort))},
			corev1.EnvVar{Name: "LISTEN_PORT", Value: strconv.Itoa(listenPortNumber)}}
		sidecar.Ports[0].ContainerPort = int32(listenPortNumber)
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, corev1.VolumeMount{Name: volumeMountName,
			MountPath: "/etc/nginx/.htpasswd", SubPath: "auth"})
		sidecar.ImagePullPolicy = corev1.PullAlways

		secretVolume := corev1.Volume{Name: volumeMountName, VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: secretName}}}

		var patch []jsonPatchElement
		patch = append(patch, jsonPatchElement{OP: "remove",
			Path: fmt.Sprintf("/spec/containers/%v/ports/%v", upstreamContainerIndex, upstreamPortIndex)})
		patch = append(patch, jsonPatchElement{OP: "add",
			Path:  fmt.Sprintf("/spec/containers/%v", len(pod.Spec.Containers)),
			Value: sidecar})
		patch = append(patch, jsonPatchElement{OP: "add",
			Path:  fmt.Sprintf("/spec/volumes/%v", len(pod.Spec.Volumes)),
			Value: secretVolume})

		patchRaw, err := json.Marshal(patch)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal patch: %v", err)
		}

		reviewResponse.Patch = patchRaw
		reviewResponse.PatchType = &jsonPatch
	}

	return &reviewResponse, nil
}

func MutatingAdmissionRequestHandler(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Error(err)
		reviewResponse.Result = &metav1.Status{Message: err.Error()}
	} else {
		reviewResponse, err = mutate(ar)
		if err != nil {
			glog.Errorf("Error mutating pod: %v", err)
		}
	}

	response := v1beta1.AdmissionReview{}
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = ar.Request.UID
	}

	// Required to not have the apiserver crash with an NPE on older versions
	// https://github.com/kubernetes/apiserver/commit/584fe98b6432033007b686f1b8063e05d20d328d
	if response.Response == nil {
		response.Response = &v1beta1.AdmissionResponse{}
	}
	// reset the Object and OldObject, they are not needed in a response.
	ar.Request.Object = runtime.RawExtension{}
	ar.Request.OldObject = runtime.RawExtension{}

	resp, err := json.Marshal(response)
	if err != nil {
		glog.Error(err)
	}
	if _, err := w.Write(resp); err != nil {
		glog.Error(err)
	}
}

func getUpstreamPortFromPod(pod corev1.Pod) (*corev1.ContainerPort, error) {
	ports := getPortsFromPod(pod)
	if upstreamPortNameFromAnnotation, exists := pod.Annotations[annotationPortName]; exists {
		upstreamPort, err := getPortByName(ports, upstreamPortNameFromAnnotation)
		if err != nil {
			return nil, err
		}
		return upstreamPort, nil
	}

	if len(ports) != 1 {
		return nil, fmt.Errorf("port name must be either passed via annotation or there must be exactly one port")
	}
	return &ports[0], nil
}

func getUnusedContainerPort(pod corev1.Pod) int {
	ports := getPortsFromPod(pod)
	candidatePort := int32(80)
	for _, port := range ports {
		if candidatePort == port.ContainerPort {
			candidatePort += candidatePort
		}
	}
	return int(candidatePort)
}

func getContainerAndPortIndexForNamedPort(pod corev1.Pod, portName string) (int, int, error) {
	for containerIndex, container := range pod.Spec.Containers {
		for portIndex, port := range container.Ports {
			if port.Name == portName {
				return containerIndex, portIndex, nil
			}
		}
	}

	return 0, 0, fmt.Errorf("no port with name %s found", portName)
}

func getPortsFromPod(pod corev1.Pod) (ports []corev1.ContainerPort) {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			ports = append(ports, port)
		}
	}

	return ports
}

func getPortByName(ports []corev1.ContainerPort, name string) (returnPort *corev1.ContainerPort, err error) {
	var alreadyFound bool
	for _, port := range ports {
		if port.Name == name {
			if alreadyFound {
				return nil, fmt.Errorf("there is more than one port with name %s!", name)
			}
			returnPort = port.DeepCopy()
		}
	}
	if returnPort == nil {
		return returnPort, fmt.Errorf("no port with name %s found!", name)
	}
	return returnPort, nil
}
