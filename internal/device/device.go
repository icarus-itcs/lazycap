package device

// Device represents a mobile device, emulator, or web target
type Device struct {
	ID         string
	Name       string
	Platform   string // "android", "ios", or "web"
	Online     bool
	IsEmulator bool
	IsWeb      bool   // true for web dev server
	APILevel   string // Android API level
	OSVersion  string // iOS version
}

// String returns a display string for the device
func (d Device) String() string {
	status := "offline"
	if d.Online {
		status = "online"
	}
	return d.Name + " (" + d.Platform + ", " + status + ")"
}
