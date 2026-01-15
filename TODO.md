# SFP Wizard Flasher - TODO

## Project Overview
Go CLI tool for interacting with the SFP Wizard (Ubiquiti) hardware device over Bluetooth Low Energy (BLE).

**Target Device:** SFP Wizard (UACC-SFP-Wizard)
**Device Firmware:** v1.1.1
**API Version:** 1.0 (as reported by device)

---

## Confirmed Working

### Core BLE Functionality
- [x] BLE adapter initialization and device scanning
- [x] Device discovery (case-insensitive name matching)
- [x] Connection handling with service discovery
- [x] Reading device info from BLE characteristic

### API Protocol (Binary Envelope + JSON)
- [x] Binary envelope encoding with zlib compression
- [x] Binary envelope decoding (handles uncompressed responses)
- [x] Handle multiple zlib compression levels (78 01, 78 9c, etc.)
- [x] Request/response correlation via incrementing hex IDs
- [x] Sequence number in outer header matches request ID

### API Commands
- [x] `version` - Read device info from BLE characteristic
- [x] `explore` - List all BLE services and characteristics
- [x] `api-version` - GET /api/version
- [x] `stats` - GET /api/1.0/{mac}/stats (battery, signal, uptime)
- [x] `info` - GET /api/1.0/{mac} (device type, name, firmware)
- [x] `settings` - GET /api/1.0/{mac}/settings
- [x] `bt` - GET /api/1.0/{mac}/bt (bluetooth parameters)
- [x] `fw` - GET /api/1.0/{mac}/fw (firmware status)
- [x] `sif-read` - Read SFP EEPROM data (vendor, serial, wavelength, etc.)
- [x] `reboot` - Reboot the device

---

## BLE Service/Characteristic Map

```
Service: 8e60f02e-f699-4865-b83f-f40501752184 (SFP API Service)
  Handle 0x10: 9280f26c-a56f-43ea-b769-d5d732e1ac67 [write] - Write API requests
  Handle 0x11: dc272a22-43f2-416b-8fa5-63a071542fac [notify, read] - Device info JSON
  Handle 0x15: d587c47f-ac6e-4388-a31c-e6cd380ba043 [notify, read] - API responses
```

**Key Discovery:** API responses come on `d587c47f` (handle 0x15), NOT `dc272a22`!

---

## Next Steps

### SIF (Support Info File) Operations
- [x] Implement POST /api/1.0/{mac}/sif/start - Start SIF read
- [x] Implement GET /api/1.0/{mac}/sif/info/ - Check SIF status
- [x] Implement GET /api/1.0/{mac}/sif/data/ - Read SIF data chunks
- [x] Implement POST /api/1.0/{mac}/sif/abort - Abort SIF operation (used in sif-read)
- [x] Handle fragmented BLE responses for large data transfers
- [x] Parse tar archive output (contains EEPROM bins, syslog, module database)
- [x] Parse and display SFP EEPROM data (vendor, model, wavelength, etc.)

### SIF Write Operations
- [ ] Implement SIF write start
- [ ] Implement SIF write data
- [ ] Test writing custom EEPROM values

### Other Features
- [ ] POST /api/1.0/{mac}/name - Set device name
- [x] POST /api/1.0/{mac}/reboot - Reboot device
- [ ] Firmware update operations (low priority)

---

## Known Limitations

- **tinygo bluetooth on Linux** doesn't support Write with Response (only WriteWithoutResponse)
  - See: https://github.com/tinygo-org/bluetooth/issues/153
  - Fortunately, WriteWithoutResponse works for the SFP Wizard API

---

## Discoveries

### SIF Archive Structure
The SIF read returns a **tar archive** containing:
- `syslog` - Device logs (RAM, clears on reboot)
- `sfp_primary.bin` - Module read via device screen
- `sfp_secondary.bin` - Module read via API
- `qsfp_primary/secondary.bin` - QSFP slots (0xff if empty)
- `{PartNumber}.bin` - Flash database, keyed by S/N, 2 slots per unique module

### Compression
- Responses may have compression flag=0x01 but send raw data
- Check for zlib magic byte 0x78 before decompressing
- Different compression levels: 78 01 (none), 78 5e (fast), 78 9c (default), 78 da (best)

---

## Sample API Responses

### GET /api/1.0/{mac}
```json
{
  "id": "DEADBEEFCAFE",
  "type": "USFPW",
  "fwv": "1.1.1",
  "bomId": "10652-8",
  "proId": "9487-1",
  "state": "app",
  "name": "Sfp Wizard"
}
```

### GET /api/1.0/{mac}/stats
```json
{
  "battery": 71,
  "batteryV": 3.888,
  "isLowBattery": false,
  "uptime": 607849,
  "signalDbm": -55
}
```

### GET /api/1.0/{mac}/settings
```json
{
  "ch": "release",
  "name": "uacc-sfp-wizard",
  "isLedEnabled": true,
  "isHwResetBlocked": false,
  "uwsType": "us",
  "intervals": {
    "intStats": 1000
  },
  "homekitEnabled": false
}
```

### GET /api/1.0/{mac}/bt
```json
{
  "btMode": "CUSTOM",
  "intervalMin": 0,
  "intervalMax": 0,
  "timeout": 0,
  "latency": 0,
  "enableLatency": false
}
```

### GET /api/1.0/{mac}/fw
```json
{
  "hwv": 8,
  "fwv": "1.1.1",
  "isUPdating": false,
  "status": "finished",
  "progressPercent": 0,
  "remainingTime": 0
}
```

---

Last Updated: 2026-01-14
