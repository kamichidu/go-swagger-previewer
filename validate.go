package main

import (
	"encoding/json"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"
)

func validate(filename string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		err := func() error {
			rc, err := os.Open(filename)
			if err != nil {
				return err
			}
			defer rc.Close()

			resp, err := http.Post("http://online.swagger.io/validator/debug", "application/json", rc)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			var msg json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
				return err
			}
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(msg)
		}()
		if err != nil {
			log.Error(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}
