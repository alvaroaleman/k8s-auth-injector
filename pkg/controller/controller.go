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

	if secretName, exists := pod.Annotations["authinjector/basic-auth-secret-name"]; exists {
		if len(pod.Spec.Containers) != 1 || len(pod.Spec.Containers[0].Ports) != 1 {
			return nil, fmt.Errorf("can only mutate pods with exactly one container that has exactly one port defined")
		}
		upstreamPort := pod.Spec.Containers[0].Ports[0].ContainerPort
		listenPort := pod.Spec.Containers[0].Ports[0].ContainerPort + 1
		volumeMountName := fmt.Sprintf("authinjector-basic-auth-secret-%s", secretName)

		sidecar := corev1.Container{Name: basicAuthSidecarName,
			Image: basicAuthSidecarImage,
			Ports: pod.Spec.Containers[0].Ports}
		sidecar.Env = []corev1.EnvVar{corev1.EnvVar{Name: "UPSTREAM_PORT",
			Value: strconv.Itoa(int(upstreamPort))},
			corev1.EnvVar{Name: "LISTEN_PORT", Value: strconv.Itoa(int(listenPort))}}
		sidecar.Ports[0].ContainerPort = listenPort
		sidecar.VolumeMounts = append(sidecar.VolumeMounts, corev1.VolumeMount{Name: volumeMountName,
			MountPath: "/etc/nginx/.htpasswd", SubPath: "auth"})
		sidecar.ImagePullPolicy = corev1.PullAlways

		secretVolume := corev1.Volume{Name: volumeMountName, VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: secretName}}}

		var patch []jsonPatchElement
		patch = append(patch, jsonPatchElement{OP: "remove", Path: "/spec/containers/0/ports"})
		patch = append(patch, jsonPatchElement{OP: "add",
			Path:  "/spec/containers/1",
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
