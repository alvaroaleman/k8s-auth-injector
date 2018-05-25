package controller

import (
	"testing"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	podResource = metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
)

func TestMutation(t *testing.T) {
	tests := []struct {
		request []byte
		patch   string
	}{
		{
			request: []byte(`{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "annotations": {
      "authinjector/basic-auth-secret-name": "ba-secret"
    },
    "labels": {
      "app": "app"
    },
    "name": "app-0",
    "namespace": "app-ns"
  },
  "spec": {
    "containers": [
      {
        "image": "quay.io/coreos/app:final",
        "name": "app",
        "ports": [
          {
            "containerPort": 2379,
            "name": "app-port",
            "protocol": "TCP"
          }
        ]
      }
    ]
  }
}`),
			patch: `[{"op":"remove","path":"/spec/containers/0/ports/0"},{"op":"add","path":"/spec/containers/1","value":{"name":"auth-sidecar","image":"docker.io/alvaroaleman/k8s-auth-injector-sidecar","ports":[{"name":"app-port","containerPort":80,"protocol":"TCP"}],"env":[{"name":"UPSTREAM_PORT","value":"2379"},{"name":"LISTEN_PORT","value":"80"}],"resources":{},"volumeMounts":[{"name":"authinjector-basic-auth-secret-ba-secret","mountPath":"/etc/nginx/.htpasswd","subPath":"auth"}],"imagePullPolicy":"Always"}},{"op":"add","path":"/spec/volumes/0","value":{"name":"authinjector-basic-auth-secret-ba-secret","secret":{"secretName":"ba-secret"}}}]`,
		},
		{
			request: []byte(`{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "annotations": {
      "authinjector/basic-auth-secret-name": "ba-secret",
      "authinjector/port-name": "app-port"
    },
    "labels": {
      "app": "app"
    },
    "name": "app-0",
    "namespace": "app-ns"
  },
  "spec": {
    "containers": [
      {
        "image": "quay.io/coreos/app:final",
        "name": "app",
        "ports": [
          {
            "containerPort": 2379,
            "name": "app-port",
            "protocol": "TCP"
          }
        ]
      },
      {
        "image": "quay.io/coreos/app:beta",
        "name": "app-beta",
        "ports": [
          {
            "containerPort": 2380,
            "name": "app-beta-port",
            "protocol": "TCP"
          }
        ]
      }
    ]
  }
}`),
			patch: `[{"op":"remove","path":"/spec/containers/1/ports/0"},{"op":"add","path":"/spec/containers/2","value":{"name":"auth-sidecar","image":"docker.io/alvaroaleman/k8s-auth-injector-sidecar","ports":[{"name":"app-port","containerPort":80,"protocol":"TCP"}],"env":[{"name":"UPSTREAM_PORT","value":"2380"},{"name":"LISTEN_PORT","value":"80"}],"resources":{},"volumeMounts":[{"name":"authinjector-basic-auth-secret-ba-secret","mountPath":"/etc/nginx/.htpasswd","subPath":"auth"}],"imagePullPolicy":"Always"}},{"op":"add","path":"/spec/volumes/0","value":{"name":"authinjector-basic-auth-secret-ba-secret","secret":{"secretName":"ba-secret"}}}]`,
		},
	}

	for _, test := range tests {
		request := v1beta1.AdmissionReview{Request: &v1beta1.AdmissionRequest{Object: runtime.RawExtension{Raw: test.request}, Resource: podResource}}
		response, err := mutate(request)
		if err != nil {
			t.Fatalf("Expected err to be nil but was: %v", err)
		}
		if response.Patch == nil {
			response.Patch = []byte("")
		}
		if string(response.Patch) != test.patch {
			t.Errorf("Expected response patch \n`%s`\n to be \n`%s`", response.Patch, test.patch)
		}
		if !response.Allowed {
			t.Errorf("Expected response to always be allowed!")
		}
	}
}
