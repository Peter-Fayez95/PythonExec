package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os/exec"
    "sync"
	"log"
	"strings"
    "github.com/google/uuid"
    "time"
)

var sessions = make(map[string]*PythonSession)

type PythonSession struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser
    stderr io.ReadCloser
    mutex  *sync.Mutex
}

type Request struct {
    ID     *string `json:"id"`
    Code   *string `json:"code"`
}

type Response struct {
    ID     string `json:"id"`
    Stdout string `json:"stdout"`
    Stderr string `json:"stderr"`
}

func NewPythonSession() (*PythonSession, error) {
    cmd := exec.Command("python3", "-i")
    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, err
    }
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, err
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
        return nil, err
    }

    if err := cmd.Start(); err != nil {
        return nil, err
    }

    ps := &PythonSession{
        cmd:    cmd,
        stdin:  stdin,
        stdout: stdout,
        stderr: stderr,
        mutex:  &sync.Mutex{},
    }

    // Step 1: Handle stdout - Write marker and consume output until __END_OF_EXECUTION__
    _, err = fmt.Fprintf(ps.stdin, "import sys; res = sys.stdout.write('__END_OF_EXECUTION__\\n'); sys.stdout.flush()\n")
    if err != nil {
        return nil, err
    }

    scannerStdout := bufio.NewScanner(ps.stdout)
    for scannerStdout.Scan() {
        if strings.Contains(scannerStdout.Text(), "__END_OF_EXECUTION__") {
			break
		}
    }

    if err := scannerStdout.Err(); err != nil {
        return nil, err
    }

    // Step 2: Handle stderr - Write marker and consume output until __END_OF_EXECUTION__
    _, err = fmt.Fprintf(ps.stdin, "import sys; res = sys.stderr.write('__END_OF_EXECUTION__\\n'); sys.stderr.flush()\n")
    if err != nil {
        return nil, err
    }

    scannerStderr := bufio.NewScanner(ps.stderr)
    for scannerStderr.Scan() {
        if strings.Contains(scannerStderr.Text(), "__END_OF_EXECUTION__") {
			break
		}
        // if scannerStderr.Text() == "__END_OF_EXECUTION__" {
        //     break
        // }
    }
    if err := scannerStderr.Err(); err != nil {
        return nil, err
    }

    // log.Println("Python session started")

    return ps, nil
}

func (ps *PythonSession) Execute(code string) (string, string, error) {
    ps.mutex.Lock()
    defer ps.mutex.Unlock()

    // Write the code followed by a newline
    _, err := fmt.Fprintf(ps.stdin, "%s\n", code)
    if err != nil {
        return "", "", err
    }

    // Write the stdout marker command
    _, err = fmt.Fprintf(ps.stdin, "import sys; res = sys.stdout.write('__END_OF_EXECUTION__\\n'); sys.stdout.flush()\n")
    if err != nil {
        return "", "", err
    }

    // Write the stderr marker command
    _, err = fmt.Fprintf(ps.stdin, "res = sys.stderr.write('__END_OF_EXECUTION__\\n'); sys.stderr.flush()\n")
    if err != nil {
        return "", "", err
    }

    var stdoutLines []string
    var stderrLines []string
    
    // Channels to collect output
    stdoutCh := make(chan string, 100)
    stderrCh := make(chan string, 100)
    stdoutDone := make(chan struct{})
    stderrDone := make(chan struct{})
    
    // Scan stdout in a goroutine
    go func() {
        scannerStdout := bufio.NewScanner(ps.stdout)
        for scannerStdout.Scan() {
            line := scannerStdout.Text()
            if strings.Contains(line, "__END_OF_EXECUTION__") {
                close(stdoutDone)
                return
            }
            stdoutCh <- line
        }
    }()
    
    // Scan stderr in a goroutine
    go func() {
        scannerStderr := bufio.NewScanner(ps.stderr)
        for scannerStderr.Scan() {
            line := scannerStderr.Text()
            if strings.Contains(line, "__END_OF_EXECUTION__") {
                close(stderrDone)
                return
            }
            stderrCh <- line
        }
    }()
    
    // Set a deadline for the execution
    timeout := time.After(2 * time.Second)
    
    // Wait for completion or timeout
    stdoutComplete := false
    stderrComplete := false
    
    for !stdoutComplete || !stderrComplete {
        select {
        case line := <-stdoutCh:
            stdoutLines = append(stdoutLines, line)
        case line := <-stderrCh:
            stderrLines = append(stderrLines, line)
        case <-stdoutDone:
            stdoutComplete = true
        case <-stderrDone:
            stderrComplete = true
        case <-timeout:
            // If timeout, consider both streams complete
            return "", "", fmt.Errorf("execution timeout")
        }
    }

    return strings.Join(stdoutLines, "\n"), strings.Join(stderrLines, "\n"), nil
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

    var session *PythonSession
    var ID string

    if req.ID == nil {
        err := error(nil)
        session, err = NewPythonSession()
        ID = uuid.New().String()
        sessions[ID] = session
        
        if err != nil {
            http.Error(w, `{"error": "Failed to start Python session"}`, http.StatusInternalServerError)
            return
        }
    } else {
        ID = *req.ID
        session = sessions[ID]
        if session == nil {
            http.Error(w, `{"error": "Invalid session ID"}`, http.StatusBadRequest)
            return
        }
    }

    stdout, stderr, err := session.Execute(*req.Code)

    if err != nil {
        if err.Error() == "execution timeout" {
            sessions[ID] = nil
            http.Error(w, `{"error": "Execution timeout and session terminated"}`, http.StatusRequestTimeout)
            return
        }
        http.Error(w, `{"error": "Execution error"}`, http.StatusInternalServerError)
        return
    }

    response := Response{
        ID:     ID,
        Stdout: stdout,
        Stderr: stderr,
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
	if err := http.ListenAndServe("0.0.0.0" + port, nil); err != nil {
		log.Fatal("Server failed to start: ", err)
	}
}