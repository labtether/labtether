package resources

import (
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/servicehttp"
)

// Not-found words for the path a file request names.
const (
	missingFile   = "file not found: "
	missingFolder = "folder not found: "
	missingParent = "parent folder not found: "
)

// goPathOps are the operation names Go puts in front of a file error, as in
// "openat docs/a.txt: no such file or directory".
var goPathOps = []string{
	"open", "openat", "stat", "statat", "lstat", "fstatat", "readdirent", "readdir",
	"mkdir", "mkdirat", "remove", "unlinkat", "rename", "renameat", "link", "linkat",
	"readlink", "readlinkat", "chmod", "chown", "truncate", "read", "write", "close",
}

// writeAgentFileError answers a file request the agent refused. Agents send
// Go's own error text; users get plain words that name the path they asked
// for. A missing path is a 404 so clients can tell it from a bad request.
func writeAgentFileError(w http.ResponseWriter, missing, path, agentError string) {
	status, message := agentFileErrorAnswer(missing, path, agentError)
	servicehttp.WriteError(w, status, message)
}

func agentFileErrorAnswer(missing, path, agentError string) (int, string) {
	text := strings.ToLower(agentError)
	switch {
	case containsAny(text, "no such file or directory", "cannot find the file", "cannot find the path"):
		return http.StatusNotFound, missing + path
	case containsAny(text, "permission denied", "operation not permitted", "access is denied"):
		return http.StatusBadRequest, "permission denied: " + path
	}
	for _, op := range goPathOps {
		if !strings.HasPrefix(agentError, op+" ") {
			continue
		}
		if i := strings.LastIndex(agentError, ": "); i > len(op) {
			return http.StatusBadRequest, agentError[i+2:] + ": " + path
		}
	}
	return http.StatusBadRequest, agentError
}

func containsAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}
