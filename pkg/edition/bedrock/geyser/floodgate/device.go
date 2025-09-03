package floodgate

// DeviceOS represents the operating system of a device.
type DeviceOS struct {
	ID   int
	Name string
}

// String implements fmt.Stringer.
func (d DeviceOS) String() string {
	return d.Name
}

// DeviceOS constants
// See https://github.com/GeyserMC/Geyser/blob/master/common/src/main/java/org/geysermc/floodgate/util/DeviceOs.java#L35
var (
	DeviceOSUnknown      = DeviceOS{ID: 0, Name: "Unknown"}
	DeviceOSAndroid      = DeviceOS{ID: 1, Name: "Android"}
	DeviceOSIOS          = DeviceOS{ID: 2, Name: "iOS"}
	DeviceOSMacOS        = DeviceOS{ID: 3, Name: "macOS"}
	DeviceOSAmazon       = DeviceOS{ID: 4, Name: "Amazon"}
	DeviceOSGearVR       = DeviceOS{ID: 5, Name: "Gear VR"}
	DeviceOSHololens     = DeviceOS{ID: 6, Name: "Hololens"} // Deprecated
	DeviceOSWindowsUWP   = DeviceOS{ID: 7, Name: "Windows"}
	DeviceOSWindowsX86   = DeviceOS{ID: 8, Name: "Windows x86"}
	DeviceOSDedicated    = DeviceOS{ID: 9, Name: "Dedicated"}
	DeviceOSAppleTV      = DeviceOS{ID: 10, Name: "Apple TV"}    // Deprecated
	DeviceOSPlayStation  = DeviceOS{ID: 11, Name: "PlayStation"} // All PlayStation platforms
	DeviceOSSwitch       = DeviceOS{ID: 12, Name: "Switch"}
	DeviceOSXbox         = DeviceOS{ID: 13, Name: "Xbox"}
	DeviceOSWindowsPhone = DeviceOS{ID: 14, Name: "Windows Phone"} // Deprecated
	DeviceOSLinux        = DeviceOS{ID: 15, Name: "Linux"}
)

// DeviceOSes is a list of all DeviceOSes.
var DeviceOSes = []DeviceOS{
	DeviceOSUnknown,
	DeviceOSAndroid,
	DeviceOSIOS,
	DeviceOSMacOS,
	DeviceOSAmazon,
	DeviceOSGearVR,
	DeviceOSHololens,
	DeviceOSWindowsUWP,
	DeviceOSWindowsX86,
	DeviceOSDedicated,
	DeviceOSAppleTV,
	DeviceOSPlayStation,
	DeviceOSSwitch,
	DeviceOSXbox,
	DeviceOSWindowsPhone,
	DeviceOSLinux,
}

// DeviceOSFromID returns the DeviceOS with the given ID.
func DeviceOSFromID(id int) DeviceOS {
	for _, os := range DeviceOSes {
		if os.ID == id {
			return os
		}
	}
	return DeviceOSUnknown
}

// IsConsole returns true if the player is using a console device.
func (d DeviceOS) IsConsole() bool {
	return d == DeviceOSSwitch ||
		d == DeviceOSXbox ||
		d == DeviceOSPlayStation
}

// IsMobile returns true if the player is using a mobile device.
func (d DeviceOS) IsMobile() bool {
	return d == DeviceOSAndroid ||
		d == DeviceOSIOS ||
		d == DeviceOSAmazon || // Fire tablets are mobile devices
		d == DeviceOSWindowsPhone
}

// IsDesktop returns true if the player is using a desktop/PC device.
func (d DeviceOS) IsDesktop() bool {
	return d == DeviceOSWindowsUWP ||
		d == DeviceOSWindowsX86 ||
		d == DeviceOSMacOS ||
		d == DeviceOSLinux
}

// IsWindows returns true if the player is using a Windows device.
func (d DeviceOS) IsWindows() bool {
	return d == DeviceOSWindowsUWP ||
		d == DeviceOSWindowsX86
}

// IsMacOS returns true if the player is using a macOS device.
func (d DeviceOS) IsMacOS() bool {
	return d == DeviceOSMacOS
}

// IsLinux returns true if the player is using a Linux device.
func (d DeviceOS) IsLinux() bool {
	return d == DeviceOSLinux
}

// IsUnknown returns true if the player is using an unknown device.
func (d DeviceOS) IsUnknown() bool {
	return d == DeviceOSUnknown
}

// IsApple returns true if the player is using an Apple device.
func (d DeviceOS) IsApple() bool {
	return d == DeviceOSMacOS ||
		d == DeviceOSIOS ||
		d == DeviceOSAppleTV
}

// IsAndroid returns true if the player is using an Android device.
// This includes Amazon Fire devices as they run Fire OS (Android-based).
func (d DeviceOS) IsAndroid() bool {
	return d == DeviceOSAndroid ||
		d == DeviceOSAmazon // Fire OS is Android-based
}

// IsAndroidBased returns true if the player is using an Android-based device.
// This includes pure Android, Amazon Fire OS, and Gear VR (which is Android-based).
func (d DeviceOS) IsAndroidBased() bool {
	return d == DeviceOSAndroid ||
		d == DeviceOSAmazon || // Fire OS is Android-based
		d == DeviceOSGearVR // Gear VR runs on Android
}
