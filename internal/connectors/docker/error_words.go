package docker

import (
	"encoding/json"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
)

// The agent reports a refused Docker Engine call as
// "docker API [METHOD] <path> returned <code>[: <body>]".
var dockerAPIErrorPattern = regexp.MustCompile(`(?s)^docker API (?:(GET|POST|PUT|DELETE|HEAD) )?(/\S*) returned (\d{3})(?::\s*(.*))?$`)

// dockerFailureWords turns an agent's Docker Engine error into what Docker
// said, without the API route or status code. Other text is kept as it is.
func dockerFailureWords(raw string) string {
	match := dockerAPIErrorPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return raw
	}
	method, path, body := match[1], match[2], strings.TrimSpace(match[4])
	code, _ := strconv.Atoi(match[3])
	var reply struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &reply) == nil && strings.TrimSpace(reply.Message) != "" {
		return sentence(reply.Message)
	}
	kind, name := dockerObjectFromPath(path)
	switch code {
	case http.StatusNotModified:
		return "Nothing changed: the " + kind + " was already in that state."
	case http.StatusNotFound:
		return "Docker has no " + kind + " named " + name + "."
	case http.StatusConflict:
		if method == http.MethodDelete {
			return "Docker did not remove the " + kind + " " + name + " because it is in use."
		}
		return "Docker refused the change because the " + kind + " " + name + " is busy."
	case http.StatusUnauthorized, http.StatusForbidden:
		return "Docker refused the request."
	default:
		return "Docker reported an error."
	}
}

// dockerObjectFromPath names the object of an Engine API path, such as
// ("image", "nginx:1.27") for /images/nginx:1.27?force=false.
func dockerObjectFromPath(path string) (string, string) {
	path = strings.SplitN(path, "?", 2)[0]
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 && strings.HasPrefix(parts[0], "v1.") {
		parts = parts[1:]
	}
	kinds := map[string]string{"containers": "container", "images": "image", "volumes": "volume", "networks": "network"}
	if len(parts) < 2 || kinds[parts[0]] == "" {
		return "object", strings.Join(parts, "/")
	}
	name := strings.Join(parts[1:], "/")
	if parts[0] != "images" && len(parts) > 2 {
		name = parts[1]
	}
	if unescaped, err := neturl.PathUnescape(name); err == nil {
		name = unescaped
	}
	if parts[0] == "images" {
		name = strings.TrimSuffix(strings.TrimSuffix(name, "/json"), "/push")
	}
	return kinds[parts[0]], name
}

func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	text = strings.ToUpper(text[:1]) + text[1:]
	if !strings.HasSuffix(text, ".") {
		text += "."
	}
	return text
}
