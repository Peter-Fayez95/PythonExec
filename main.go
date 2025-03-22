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

    // Consume initial startup messages
    // Write the marker command
    _, err = fmt.Fprintf(ps.stdin, "import sys; sys.stdout.write('__PROMPT__\\n'); sys.stdout.flush()\n")
    if err != nil {
        return nil, err
    }

    _, err = fmt.Fprintf(ps.stdin, "import sys; sys.stderr.write('__PROMPT__\\n'); sys.stderr.flush()\n")
    if err != nil {
        return nil, err
    }


    scannerStdout := bufio.NewScanner(ps.stdout)
    // scannerStderr := bufio.NewScanner(ps.stderr)
    for scannerStdout.Scan() {
        if scannerStdout.Text() == "__PROMPT__" {
            break
        }
    }

    // for scannerStderr.Scan() {
    //     if scannerStderr.Text() == "__PROMPT__" {
    //         break
    //     }
    // }


    if err := scannerStdout.Err(); err != nil {
        return nil, err
    }

    // if err := scannerStderr.Err(); err != nil {
    //     return nil, err
    // }


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

    // Write the marker command
    _, err = fmt.Fprintf(ps.stdin, "import sys; sys.stdout.write('__PROMPT__\\n'); sys.stdout.flush()\n")
    if err != nil {
        return "", "", err
    }
    // _, err = fmt.Fprintf(ps.stdin, "import sys; sys.stderr.write('__PROMPT__\\n'); sys.stderr.flush()\n")
    // if err != nil {
    //     return "", "", err
    // }

    log.Printf("Executing code: %s", code)

    // Read output until the marker
    scannerStdout := bufio.NewScanner(ps.stdout)
    // scannerStderr := bufio.NewScanner(ps.stderr)

    var stdoutLines []string 
    // var stderrLines []string
    
    for scannerStdout.Scan() {
        line := scannerStdout.Text()
        if line == "__PROMPT__" {
            break
        }
        stdoutLines = append(stdoutLines, line)
    }
    log.Printf("stdoutLines: %s", stdoutLines)

    // for scannerStderr.Scan() {
    //     line := scannerStderr.Text()
    //     if line == "__PROMPT__" {
    //         break
    //     }
    //     stderrLines = append(stderrLines, line)
    // }

    if err := scannerStdout.Err(); err != nil {
        return "", "", err
    }

    // if err := scannerStderr.Err(); err != nil {
    //     return "", "", err
    // }

    // Combine output lines
    return strings.Join(stdoutLines, "\n"), "", nil
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
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start: ", err)
	}
}