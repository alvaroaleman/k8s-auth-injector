# k8s-auth-injector

## Description

A `MutatingAdmissionWebhook` that adds a sidecar for authentication to your pod.

Right now only basic auth is supported, but adding other mechanisms can be easely done.

It works by reading a `authinjector/basic-auth-secret-name` annotation on the pod, then
adding a sidecar which uses the `secret` specified in the annotation. That sidecar then redefines the port it puts
basic auth on to point to the sidecar. This means your service should point to the name of the port, not to the
number.

If there is more than one port defined in your pod, you must also set a `authinjector/port-name` with the
name of the port you want basic auth on

*Note:* This requires the secret specified in `authinjector/basic-auth-secret-name` to have a `auth` property with the
basic auth infos

## Quickstart

1. `make webhook`: Generates certificates and deploys the webhook and a `MutatingWebhookConfiguration`
2. `htpasswd -c auth foo`: Generate a basic auth file
3. `kubectl create secret generic basic-auth --from-file=auth`: Create a secret from the basic auth file
4. `kubectl apply -f manifests/echoheaders.yaml`: Deploy the echo headers app to see the hook in action
