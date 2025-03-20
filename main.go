package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	// "bufio"
	// "sync"
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

var (
	// Create Python subprocess
	cmd = exec.Command("python3", "-q", "-u", "-i")

	// Get stdin pipe
	stdin, errStdin = cmd.StdinPipe()

	// Get stdout pipe
	stdout, errStdout = cmd.StdoutPipe()

	// Get stderr pipe
	stderr, errStderr = cmd.StderrPipe()
)

func ReadfromReader(r io.Reader) string {
	buffer := io.LimitReader(r, 1)
	log.Println("Reading from buffer")
	buf := make([]byte, 80)
	n, err := buffer.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	return string(buf[:n])
}

func execHandler(w http.ResponseWriter, r *http.Request) {
	// Start the command
	cmd.Start()

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

	log.Printf("Received request: %s", *req.Code)

	if errStdout != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to create stdout pipe"}`, http.StatusInternalServerError)
		return
	}

	if errStderr != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to create stderr pipe"}`, http.StatusInternalServerError)
		return
	}
	
	_, err := io.WriteString(stdin, *req.Code + "\n")
	if err != nil {
		// Return error and omit stdout/stderr
		http.Error(w, `{"error": "Failed to write to stdin"}`, http.StatusInternalServerError)
		return
	}
	log.Println("Wrote to stdin")

	// cmd.Wait()
	// log.Println(*stdout)

	// var wg sync.WaitGroup

	// wg.Add(2)
	var stdoutBytes, stderrBytes string
	// go func() {
		// log.Println("Reading from stdout")
		// defer wg.Done()
		stdoutBytes = ReadfromReader(stdout)
	// }()

	// go func() {
	// 	log.Println("Reading from stderr")
	// 	defer wg.Done()
		stderrBytes = ReadfromReader(stderr)
	// }()
	
	// wg.Wait()
	// log.Println("Read from stdout and stderr")
	
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

	
	// if err := cmd.Start(); err != nil {
	// 	// Return error and omit stdout/stderr
	// 	http.Error(w, `{"error": "Failed to start Python process"}`, http.StatusInternalServerError)
	// 	return
	// }

	// Start the server and listen for requests
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start: ", err)
	}
}
