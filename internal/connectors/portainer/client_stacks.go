package portainer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// stackEnv is one environment variable of a stack. Values are often secrets
// (database passwords, API keys): they stay inside this file, are only sent
// back to Portainer on an update, and are never returned, logged or put in
// an error.
type stackEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// stackDetail is a stack as an update needs it. It is private on purpose:
// the shared Stack type is what the Hub returns to the console, and must not
// carry the environment.
type stackDetail struct {
	Stack
	Env []stackEnv `json:"Env"`
}

// getStackDetail reads one stack with how it was deployed and its environment.
func (c *Client) getStackDetail(ctx context.Context, stackID int) (stackDetail, error) {
	payload, err := c.get(ctx, fmt.Sprintf("/api/stacks/%s", url.PathEscape(fmt.Sprintf("%d", stackID))))
	if err != nil {
		return stackDetail{}, err
	}
	var stack stackDetail
	if err := json.Unmarshal(payload, &stack); err != nil {
		return stackDetail{}, fmt.Errorf("decode stack %d response", stackID)
	}
	if stack.Env == nil {
		// Portainer clears the variables of an update that sends none.
		stack.Env = []stackEnv{}
	}
	return stack, nil
}

// RedeployStack deploys a stack again, optionally pulling new images. A stack
// deployed from git is pulled from its repository again. Any other stack (made
// in Portainer's editor or uploaded) has no repository; it is deployed again
// from its stored compose file. Both keep the stack's environment variables.
func (c *Client) RedeployStack(ctx context.Context, stackID, endpointID int, pullImage bool) error {
	stack, err := c.getStackDetail(ctx, stackID)
	if err != nil {
		return err
	}
	if stack.GitConfig != nil {
		path := fmt.Sprintf("/api/stacks/%s/git/redeploy?endpointId=%d", url.PathEscape(fmt.Sprintf("%d", stackID)), endpointID)
		_, err = c.put(ctx, path, map[string]any{"pullImage": pullImage, "env": stack.Env, "prune": false})
		return err
	}
	content, err := c.GetStackCompose(ctx, stackID)
	if err != nil {
		return err
	}
	return c.updateStack(ctx, stack, endpointID, content, pullImage)
}

// UpdateStackCompose replaces a stack's compose file and deploys it, keeping
// the stack's environment variables.
func (c *Client) UpdateStackCompose(ctx context.Context, stackID, endpointID int, composeContent string) error {
	stack, err := c.getStackDetail(ctx, stackID)
	if err != nil {
		return err
	}
	return c.updateStack(ctx, stack, endpointID, composeContent, false)
}

func (c *Client) updateStack(ctx context.Context, stack stackDetail, endpointID int, content string, pullImage bool) error {
	path := fmt.Sprintf("/api/stacks/%s?endpointId=%d", url.PathEscape(fmt.Sprintf("%d", stack.ID)), endpointID)
	body := map[string]any{
		"StackFileContent": content,
		"Env":              stack.Env,
		"Prune":            false,
		"PullImage":        pullImage,
	}
	_, err := c.put(ctx, path, body)
	return err
}
