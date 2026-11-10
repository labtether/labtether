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

// writeAgentFileMoveError answers a rename or copy the agent refused. A name
// clash is about the destination; every other error is about the source.
func writeAgentFileMoveError(w http.ResponseWriter, srcPath, dstPath, agentError string) {
	status, message := agentFileMoveErrorAnswer(srcPath, dstPath, agentError)
	servicehttp.WriteError(w, status, message)
}

func agentFileMoveErrorAnswer(srcPath, dstPath, agentError string) (int, string) {
	// "destination already exists" is the agent's own check; the other two are
	// the Linux and Windows words for a name taken after that check.
	if containsAny(strings.ToLower(agentError), "destination already exists", "file exists", "file already exists") {
		return http.StatusConflict, nameTakenText(dstPath)
	}
	return agentFileErrorAnswer(missingFile, srcPath, agentError)
}

// nameTakenText names the entry and its folder, as in
// `"two.txt" already exists in /srv/docs. Choose another name.`
func nameTakenText(path string) string {
	folder, name := splitAgentPath(path)
	if folder == "" {
		return `"` + name + `" already exists. Choose another name.`
	}
	return `"` + name + `" already exists in ` + folder + ". Choose another name."
}

// splitAgentPath splits a path from any agent OS into its folder and last
// name. A backslash separates names unless the path starts with "/", where it
// can be part of a name.
func splitAgentPath(path string) (folder, name string) {
	separators := `/\`
	if strings.HasPrefix(path, "/") {
		separators = "/"
	}
	trimmed := strings.TrimRight(path, separators)
	i := strings.LastIndexAny(trimmed, separators)
	switch {
	case trimmed == "":
		return "", path
	case i < 0:
		return "", trimmed
	case i == 0 || trimmed[i-1] == ':':
		return trimmed[:i+1], trimmed[i+1:]
	}
	return trimmed[:i], trimmed[i+1:]
}

func containsAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}
