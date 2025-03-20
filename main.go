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
	Code string `json:"code"`
}

type Response struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func execHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set response content type
	w.Header().Set("Content-Type", "application/json")

	// Decode the request body
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create Python subprocess
	cmd := exec.Command("python3", "-c", req.Code)
	
	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "Failed to create stdout pipe", http.StatusInternalServerError)
		return
	}
	
	// Get stderr pipe
	stderr, err := cmd.StderrPipe()
	if err != nil {
		http.Error(w, "Failed to create stderr pipe", http.StatusInternalServerError)
		return
	}
	
	// Start the command
	if err := cmd.Start(); err != nil {
		http.Error(w, "Failed to start Python process", http.StatusInternalServerError)
		return
	}
	
	// Read stdout
	stdoutBytes, err := io.ReadAll(stdout)
	if err != nil {
		http.Error(w, "Failed to read stdout", http.StatusInternalServerError)
		return
	}
	
	// Read stderr
	stderrBytes, err := io.ReadAll(stderr)
	if err != nil {
		http.Error(w, "Failed to read stderr", http.StatusInternalServerError)
		return
	}
	
	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		// We don't return an error here because we want to return the stderr output
		// even if the Python code had an error
		log.Printf("Python execution error: %v", err)
	}
	
	// Create response
	response := Response{
		Stdout: string(stdoutBytes),
		Stderr: string(stderrBytes),
	}
	
	// Write the JSON response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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