FROM alpine


COPY k8s-auth-injector /usr/local/bin

USER nobody

CMD ["/usr/local/bin/k8s-auth-injector", \
     "-tls-cert-file", "/etc/k8s-auth-injector/serve.crt", \
     "-tls-private-key-file", "/etc/k8s-auth-injector/serve.key", \
     "-logtostderr", \
     "-v", "4"]
