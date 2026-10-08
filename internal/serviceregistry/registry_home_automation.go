package serviceregistry

// knownHomeAutomationServices owns the home automation catalog.
func knownHomeAutomationServices() map[string]KnownService {
	return map[string]KnownService{
		"homeassistant": {
			Key: "homeassistant", Name: "Home Assistant", Category: CatHomeAutomation, IconKey: "homeassistant",
			DefaultPort: 8123, HealthPath: "/api/",
			DockerImages: []string{"ghcr.io/home-assistant/home-assistant", "homeassistant/home-assistant"},
			Description:  "Open-source home automation platform",
		},
		"nodered": {
			Key: "nodered", Name: "Node-RED", Category: CatHomeAutomation, IconKey: "nodered",
			DefaultPort: 1880, HealthPath: "/",
			DockerImages: []string{"nodered/node-red"},
			Description:  "Low-code programming for event-driven automation",
		},
		"mqtt": {
			Key: "mqtt", Name: "Mosquitto MQTT", Category: CatHomeAutomation, IconKey: "mqtt",
			DefaultPort: 1883, HealthPath: "",
			DockerImages: []string{"eclipse-mosquitto"},
			Description:  "Lightweight MQTT message broker",
		},
		"zigbee2mqtt": {
			Key: "zigbee2mqtt", Name: "Zigbee2MQTT", Category: CatHomeAutomation, IconKey: "zigbee2mqtt",
			DefaultPort: 8080, HealthPath: "/api/health",
			DockerImages: []string{"koenkk/zigbee2mqtt"},
			Description:  "Zigbee to MQTT bridge",
		},
		"esphome": {
			Key: "esphome", Name: "ESPHome", Category: CatHomeAutomation, IconKey: "esphome",
			DefaultPort: 6052, HealthPath: "/",
			DockerImages: []string{"ghcr.io/esphome/esphome", "esphome/esphome"},
			Description:  "ESP8266/ESP32 device configuration and management",
		},
		"scrypted": {
			Key: "scrypted", Name: "Scrypted", Category: CatHomeAutomation, IconKey: "scrypted",
			DefaultPort: 10443, HealthPath: "/",
			DockerImages: []string{"koush/scrypted"},
			Description:  "Home video integration and automation platform",
		},
		"deconz": {
			Key: "deconz", Name: "deCONZ", Category: CatHomeAutomation, IconKey: "deconz",
			DefaultPort: 80, HealthPath: "/api/config",
			DockerImages: []string{"deconzcommunity/deconz"},
			Description:  "Zigbee gateway and network manager via ConBee/RaspBee",
		},
		"zwavejs": {
			Key: "zwavejs", Name: "Z-Wave JS UI", Category: CatHomeAutomation, IconKey: "zwave-js-ui",
			DefaultPort: 8091, HealthPath: "/health",
			DockerImages: []string{"zwavejs/zwave-js-ui"},
			Description:  "Z-Wave network management and MQTT bridge",
		},
		"wyze-bridge": {
			Key: "wyze-bridge", Name: "Wyze Bridge", Category: CatHomeAutomation, IconKey: "wyze",
			DefaultPort: 5000, HealthPath: "/",
			DockerImages: []string{"mrlt8/wyze-bridge"},
			Description:  "RTSP/WebRTC bridge for Wyze camera streams",
		},
		"double-take": {
			Key: "double-take", Name: "Double Take", Category: CatHomeAutomation, IconKey: "double-take",
			DefaultPort: 3000, HealthPath: "/api/config",
			DockerImages: []string{"jakowenko/double-take"},
			Description:  "Unified facial recognition for home automation",
		},
	}
}
