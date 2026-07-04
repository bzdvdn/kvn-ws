package main

import (
	"net/http"
	"time"
)

// @sk-task kvn-desktop#T2.1: check if kvn-web is reachable (AC-006)
func checkServer(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/api/platform")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
