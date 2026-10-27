package shared

import "errors"

// HubSSHKeyOffMessage says why the Hub will not add or use its SSH key on a
// device, and how to change that.
const HubSSHKeyOffMessage = "Hub SSH key is off for this device. An admin or owner can turn it on in the device's Connect panel."

// ErrHubSSHKeyOff is returned when a connection would log in with the Hub SSH
// key on a device where an admin has not turned the key on.
var ErrHubSSHKeyOff = errors.New(HubSSHKeyOffMessage)
