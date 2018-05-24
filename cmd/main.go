package main

import (
	"crypto/tls"
	"flag"
	"net/http"

	"github.com/alvaroaleman/k8s-auth-injector/pkg/controller"

	"github.com/golang/glog"
)

// Config contains the tls certificate
type Config struct {
	CertFile string
	KeyFile  string
}

func configTLS(config Config) *tls.Config {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		glog.Fatal(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}
}

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "tls-cert-file", c.CertFile, ""+
		"File containing the tls certificate")
	flag.StringVar(&c.KeyFile, "tls-private-key-file", c.KeyFile, ""+
		"File containing the default tls private key matching --tls-cert-file.")
}

func main() {
	glog.V(2).Info("Starting...")
	var config Config
	config.addFlags()
	flag.Parse()

	ch := make(chan int)
	http.HandleFunc("/", controller.MutatingAdmissionRequestHandler)
	server := &http.Server{
		Addr:      ":8443",
		TLSConfig: configTLS(config),
	}

	go func() {
		err := server.ListenAndServeTLS("", "")
		if err != nil {
			glog.Fatalf("Error starting server: '%v'", err)
		}
	}()

	glog.V(2).Info("Startup completed")
	<-ch
	glog.V(2).Info("Shutting down..")
}
