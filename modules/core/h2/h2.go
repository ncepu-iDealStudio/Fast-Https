package h2

import (
	"fmt"
	"log"
	"net/http"
)

func H2() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, HTTP/2!")
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: nil,
	}

	err := server.ListenAndServeTLS("./config/cert/localhost.pem", "./config/cert/localhost-key.pem")
	if err != nil {
		log.Fatal(err)
	}
}
