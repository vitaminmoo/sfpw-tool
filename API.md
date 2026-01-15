# UACC SFP Wizard BLE API Protocol

This document describes the BLE API protocol for the UACC SFP Wizard device (firmware version 1.1.1).

## Protocol Overview

The protocol uses **JSON messages over BLE GATT**. Messages are wrapped in a JSON envelope with metadata.

### Message Format

#### Request Envelope
```json
{
  "id": "<uuid>",
  "timestamp": <unix_timestamp_ms>,
  "method": "<path>",
  "headers": {},
  "body": <body_object_or_null>
}
```

#### Response Envelope
```json
{
  "id": "<uuid>",
  "timestamp": <unix_timestamp_ms>,
  "statusCode": <http_status_code>,
  "headers": {},
  "body": <body_object_or_null>
}
```

### Key Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | UUID for request/response correlation |
| `timestamp` | number | Unix timestamp in milliseconds |
| `method` | string | The API path (request only) |
| `statusCode` | number | HTTP-like status code (response only) |
| `headers` | object | Optional headers (typically empty `{}`) |
| `body` | object/binary | Request or response payload |

### Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 304 | Not Modified |
| 400 | Bad Request |
| 404 | Not Found |
| 500 | Internal Server Error |

---

## MAC Address Format

The device MAC is formatted as **12 uppercase hex characters without separators**.

Example: MAC `AA:BB:CC:DD:EE:FF` becomes `AABBCCDDEEFF`

---

## API Endpoints

All endpoints (except version) use the base path: `/api/1.0/{MAC}/`

### Version Endpoints

#### GET `/api/version` or `/api/1.0/version`

Returns firmware and API version info.

**Request:**
```json
{
  "id": "...",
  "timestamp": 1234567890000,
  "method": "/api/version",
  "headers": {},
  "body": null
}
```

**Response Body:**
```json
{
  "fwv": "1.1.1",
  "apiVersion": "1.0"
}
```

---

#### GET `/api/1.0/{MAC}`

Returns full device information.

**Response Body:**
```json
{
  "id": "AABBCCDDEEFF",
  "type": "USFPW",
  "fwv": "1.1.1",
  "bomId": "1-0",
  "proId": "1-0",
  "state": "app",
  "name": "My Device"
}
```

---

### Bluetooth Status

#### GET `/api/1.0/{MAC}/bt`

Returns BLE connection parameters.

**Response Body:**
```json
{
  "btMode": "ble",
  "intervalMin": 6,
  "intervalMax": 12,
  "timeout": 100,
  "latency": 0,
  "enableLatency": false
}
```

---

### Device Settings

#### GET `/api/1.0/{MAC}/settings`

Returns device settings.

**Response Body:**
```json
{
  "ch": "release",
  "name": "uacc_sfp_wizard",
  "isLedEnabled": true,
  "isHwResetBlocked": false,
  "uwsType": "us",
  "intervals": {
    "intStats": 1000
  },
  "homekitEnabled": false
}
```

---

### Device Statistics

#### GET `/api/1.0/{MAC}/stats`

Returns device statistics.

**Response Body:**
```json
{
  "battery": 85,
  "batteryV": 3.92,
  "isLowBattery": false,
  "uptime": 3600,
  "signalDbm": -45
}
```

---

### Device Name

#### POST `/api/1.0/{MAC}/name`

Sets the device friendly name.

**Request Body:**
```json
{
  "name": "New Device Name"
}
```

**Response:** Status 200 (success), 304 (not modified), or 500 (error)

---

### Firmware Update

#### GET `/api/1.0/{MAC}/fw`

Returns firmware update status and device info.

**Response Body:**
```json
{
  "hwv": 1,
  "fwv": "1.1.1",
  "isUPdating": false,
  "status": "idle",
  "progressPercent": 0,
  "remainingTime": 0
}
```

**Status Values:** `idle`, `continue`, `complete`, `finished`, `error`

---

#### POST `/api/1.0/{MAC}/fw/start`

Initiates firmware update.

**Request Body:**
```json
{
  "size": 1048576
}
```

(size is total firmware size in bytes)

**Response Body:**
```json
{
  "status": "continue",
  "offset": 0,
  "size": 1048576
}
```

---

#### POST `/api/1.0/{MAC}/fw/data`

Sends firmware data chunk. **Body is raw binary data**, not JSON.

**Request Body:** Raw binary firmware chunk

**Response Body:**
```json
{
  "status": "continue",
  "offset": 4096,
  "size": 1048576
}
```

---

#### POST `/api/1.0/{MAC}/fw/abort`

Aborts firmware update.

**Request Body:** None (null)

**Response:** Status 200

---

### SIF (SFP Interface) Operations

The SIF endpoints control reading/writing SFP transceiver EEPROM data.

#### POST `/api/1.0/{MAC}/sif/start`

Starts a SIF read/write operation.

**Request Body:** None (null)

**Response:** Status 200 or 500

---

#### GET `/api/1.0/{MAC}/sif/info`

Returns SIF operation status.

**Response Body:**
```json
{
  "status": "continue",
  "offset": 0,
  "chunk": 64,
  "size": 256
}
```

**Status Values:** `idle`, `start`, `continue`, `complete`, `error`

---

#### POST `/api/1.0/{MAC}/sif/data`

Transfers SIF data chunk.

**Request Body:**
```json
{
  "status": "continue",
  "offset": 0,
  "chunk": 64
}
```

| Field | Description |
|-------|-------------|
| `status` | `continue` (more data), `complete` (last chunk), `error` (abort) |
| `offset` | Byte offset in SFP EEPROM |
| `chunk` | Chunk size in bytes |

**Response Body:**
```json
{
  "status": "continue",
  "offset": 64,
  "chunk": 64,
  "size": 256
}
```

---

#### POST `/api/1.0/{MAC}/sif/abort`

Aborts SIF operation.

**Request Body:** None (null)

**Response:** Status 200 or 500

---

### Reboot

#### POST `/api/1.0/{MAC}/reboot`

Reboots the device.

**Request Body:** None (null)

**Response:** Status 200, then device reboots

---

## Example Client Flow

### 1. Discover Device
Connect to BLE device advertising as "UACC-SFP-Wizard"

### 2. Get Version
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "timestamp": 1699900000000,
  "method": "/api/version",
  "headers": {},
  "body": null
}
```

### 3. Get Device Info
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440001",
  "timestamp": 1699900001000,
  "method": "/api/1.0/AABBCCDDEEFF",
  "headers": {},
  "body": null
}
```

### 4. Read SFP Data
```json
// Start SIF operation
{
  "id": "...",
  "timestamp": ...,
  "method": "/api/1.0/AABBCCDDEEFF/sif/start",
  "headers": {},
  "body": null
}

// Poll for data chunks
{
  "id": "...",
  "timestamp": ...,
  "method": "/api/1.0/AABBCCDDEEFF/sif/data",
  "headers": {},
  "body": {
    "status": "continue",
    "offset": 0,
    "chunk": 64
  }
}
// Repeat with increasing offset until status is "complete"
```

### 5. Firmware Update
```json
// Start update
{
  "id": "...",
  "timestamp": ...,
  "method": "/api/1.0/AABBCCDDEEFF/fw/start",
  "headers": {},
  "body": {"size": 1048576}
}

// Send chunks (body is raw binary, not JSON)
// Continue until all data sent

// Check status
{
  "id": "...",
  "timestamp": ...,
  "method": "/api/1.0/AABBCCDDEEFF/fw",
  "headers": {},
  "body": null
}
```

---

## Notes

1. **No explicit GET/POST distinction** - if `body` is null, it's a read operation; if body has content, it's a write operation.

2. **Binary data for /fw/data** - The firmware data endpoint expects raw binary in the body field, not JSON.

3. **MAC validation** - The device only responds to requests addressed to its own MAC address.

4. **UUID correlation** - Use the `id` field to match responses to requests.

5. **BLE MTU** - Large responses may be chunked based on BLE MTU size.

---

## Discovered Functions (Ghidra)

| Address | Function Name | Purpose |
|---------|--------------|---------|
| 0x42025bc4 | `ble_api_command_handler` | Main API router |
| 0x42025b2c | `ble_api_build_response` | Build response envelope |
| 0x42025628 | `ble_api_send_error_response` | Error response |
| 0x4207756c | `ble_send_response` | Send BLE response |
| 0x42076c08 | `json_get_string_value` | Parse JSON field |
| 0x42024c7c | `ble_api_get_device_mac` | Get device MAC (format: %02X%02X%02X%02X%02X%02X) |
| 0x42024db0 | `ble_api_handle_version` | /api/1.0/{mac} handler |
| 0x42024d1c | `ble_api_handle_version_legacy` | /api/version handler |
| 0x42024f6c | `ble_api_handle_bt` | /bt handler |
| 0x42025028 | `ble_api_handle_settings` | /settings handler |
| 0x420250d8 | `ble_api_handle_stats` | /stats handler |
| 0x42025744 | `ble_api_handle_fw_start` | /fw/start handler |
| 0x42024b74 | `ble_api_handle_fw_data` | /fw/data handler |
| 0x42024b38 | `ble_api_handle_fw_abort` | /fw/abort handler |
| 0x42025398 | `ble_api_handle_fw_info` | /fw handler |
| 0x42025238 | `ble_api_handle_fw_status` | FW status response |
| 0x420256a4 | `ble_api_handle_name` | /name handler |
| 0x42025548 | `ble_api_handle_sif_info` | /sif/info handler |
| 0x42025820 | `ble_api_handle_sif_data` | /sif/data handler |
| 0x42025b94 | `ble_api_lookup_custom_handler` | Custom endpoint lookup |
