export CGO_ENABLED := 0

k8s-auth-injector: $(shell find . -name '*.go')
			@go build \
				-ldflags '-s -w' \
				-o k8s-auth-injector \
				github.com/alvaroaleman/k8s-auth-injector/cmd

docker image: k8s-auth-injector
	docker build -t alvaroaleman/k8s-auth-injector .
	docker build -t alvaroaleman/k8s-auth-injector-sidecar sidecar/
	docker push alvaroaleman/k8s-auth-injector
	docker push alvaroaleman/k8s-auth-injector-sidecar

test:
	go test ./... -v

manifests/ca.key:
	openssl genrsa -out manifests/ca.key 4096

manifests/ca.crt: manifests/ca.key
	openssl req -x509 -new -nodes -key manifests/ca.key \
    -subj "/C=US/ST=CA/O=Acme/CN=k8s-auth-injector-ca" \
		-sha256 -days 10000 -out manifests/ca.crt

manifests/serve.key: manifests/ca.crt
	openssl genrsa -out manifests/serve.key 2048
	chmod 0600 manifests/serve.key

manifests/serve.crt: manifests/serve.key
	openssl req -new -sha256 \
    -key manifests/serve.key \
    -subj "/C=US/ST=CA/O=Acme/CN=k8s-auth-injector.kube-system.svc" \
    -out manifests/serve.csr
	openssl x509 -req -in manifests/serve.csr -CA manifests/ca.crt \
      -CAkey manifests/ca.key -CAcreateserial \
			-out manifests/serve.crt -days 10000 -sha256

.PHONY: crt
crt: manifests/serve.crt

.PHONY: secret
secret: crt
	@cd manifests && \
	kubectl get secret k8s-auth-injector-serving-certs -n kube-system &>/dev/null \
		|| kubectl create secret generic k8s-auth-injector-serving-certs \
			-n kube-system --from-file=serve.key --from-file=serve.crt

.PHONY: deployment
deployment: secret
	kubectl apply -f manifests/deployment.yaml

.PHONY: webhook
webhook: deployment
	@cat manifests/hook.yaml \
		|sed 's/<<CACERT>>/$(shell cat manifests/ca.crt |base64 -w0)/g' \
		|kubectl apply -f-
