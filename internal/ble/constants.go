package ble

const (
	// SFPServiceUUID is the primary SFP service UUID
	SFPServiceUUID = "8E60F02E-F699-4865-B83F-F40501752184"

	// SFPWriteCharUUID is the characteristic for writing API requests
	SFPWriteCharUUID = "9280F26C-A56F-43EA-B769-D5D732E1AC67"

	// SFPNotifyCharUUID is the characteristic for device info (read)
	SFPNotifyCharUUID = "DC272A22-43F2-416B-8FA5-63A071542FAC"

	// SFPSecondaryNotifyUUID is the characteristic for API responses (notify)
	SFPSecondaryNotifyUUID = "D587C47F-AC6E-4388-A31C-E6CD380BA043"

	// SFPService2UUID is the secondary service (v1.1.1)
	SFPService2UUID = "0B9676EE-8352-440A-BF80-61541D578FCF"
)
