package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
)

type Request struct {
	ID   *string `json:"id"`
	Code *string `json:"code"`
}

type Response struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Error  string `json:"error,omitempty"`
}

func execHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Set response content type
	w.Header().Set("Content-Type", "application/json")

	// Decode the request body
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Code == nil {
		http.Error(w, `{"error": "Missing required field: 'code'"}`, http.StatusBadRequest)
		return
	}

	// Create Python subprocess
	cmd := exec.Command("python3", "-c", *req.Code)

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to create stdout pipe"}`, http.StatusInternalServerError)
		return
	}

	// Get stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to create stderr pipe"}`, http.StatusInternalServerError)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to start Python process"}`, http.StatusInternalServerError)
		return
	}

	// Read stdout
	stdoutBytes, err := io.ReadAll(stdout)
	if err != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to read stdout"}`, http.StatusInternalServerError)
		return
	}

	// Read stderr
	stderrBytes, err := io.ReadAll(stderr)
	if err != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to read stderr"}`, http.StatusInternalServerError)
		return
	}

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		// Log the Python execution error but do not return it in the response
		log.Printf("Python execution error: %v", err)
	}

	// log.Printf("stdout: %s", string(stdoutBytes))
	// log.Printf("stderr: %s", string(stderrBytes))
	response := Response{
		Stdout: string(stdoutBytes),
		Stderr: string(stderrBytes),
	}
	_ = json.NewEncoder(w).Encode(response)
}

func main() {
	http.HandleFunc("/execute", execHandler)

	// Define the port where the server will listen
	port := ":8080"
	fmt.Println("Server is listening on port", port)

	// Start the server and listen for requests
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start: ", err)
	}
}
