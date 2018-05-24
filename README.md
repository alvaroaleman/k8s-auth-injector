# k8s-auth-injector

## Description

A `MutatingAdmissionWebhook` that adds a sidecar for basic authentication to your pod.

It works by reading a `authinjector/basic-auth-secret-name` annotation on the pod, then
adding a sidecar which uses the `secret` specified in the annotation.

Note:
 * This currently only works with pods that have exactly one container and that container has exactly one port
   specified
 * This requires the secret specified in `authinjector/basic-auth-secret-name` to have a `auth` property with the
   basic auth infos

## Quickstart

1. `make webhook`: Generates certificates and deploys the webhook and a `MutatingWebhookConfiguration`
2. `htpasswd -c auth foo`: Generate a basic auth file
3. `kubectl create secret generic basic-auth --from-file=auth`: Create a secret from the basic auth file
4. `kubectl apply -f manifests/echoheaders.yaml`: Deploy the echo headers app to see the hook in action
