# UACC SFP Wizard BLE API Protocol

This document describes the BLE API protocol for the UACC SFP Wizard device.

> [!WARNING] Maintained via LLM with human guidance - certainly contains errors

## Firmware Version Compatibility

Tested firmware versions: **1.0.10**, **1.1.0**, **1.1.1**, **1.1.3**

| Endpoint                             | 1.0.10 | 1.1.0 | 1.1.1+ | Notes                                                |
| ------------------------------------ | ------ | ----- | ------ | ---------------------------------------------------- |
| `/api/1.0/{mac}`                     | ✓      | ✓     | ✓      | Device info                                          |
| `/api/1.0/{mac}/stats`               | ✓      | ✓     | ✓      |                                                      |
| `/api/1.0/{mac}/settings`            | ✓      | ✓     | ✓      |                                                      |
| `/api/1.0/{mac}/bt`                  | ✓      | ✓     | ✓      |                                                      |
| `/api/1.0/{mac}/fw`                  | ✓      | ✓     | ✓      |                                                      |
| `/api/1.0/{mac}/xsfp/module/details` | 404    | ✓     | ✓      | **New in 1.1.0**, adds `type` field in 1.1.1         |
| `/api/1.0/{mac}/xsfp/sync/start`     | ✓      | ✓     | ✓      | Returns 417 if no module; adds `type` field in 1.1.1 |
| `/api/1.0/{mac}/xsfp/module/start`   | ?      | ✓     | ✓      | Untested on 1.0.10                                   |
| `/api/1.0/{mac}/sif/*`               | ✓      | ✓     | ✓      | SIF archive operations                               |

**Key changes in 1.1.0:**

- Added `/xsfp/module/details` endpoint for quick module info without full EEPROM read

**Key changes in 1.1.1:**

- Added `type` field ("sfp" or "qsfp") to `/xsfp/module/details` and `/xsfp/sync/start` responses

---

## BLE Services Overview

The device exposes four BLE services:

### Service 1: Generic Access (0x1800) - Standard

| Characteristic | UUID   | Description               |
| -------------- | ------ | ------------------------- |
| Device Name    | 0x2A00 | Returns "UACC-SFP-Wizard" |
| Appearance     | 0x2A01 | Returns 0x0000 (Generic)  |

### Service 2: Generic Attribute (0x1801) - Standard

| Characteristic            | UUID   | Description                |
| ------------------------- | ------ | -------------------------- |
| Service Changed           | 0x2A05 | Standard GATT notification |
| Client Supported Features | 0x2B3A | Client features            |
| Database Hash             | 0x2B29 | GATT database hash         |

### Service 3: Device Info & Control (8e60f02e-f699-4865-b83f-f40501752184)

Simple text-based command interface for device control.

| Characteristic | UUID                                 | Handle | Description                                    |
| -------------- | ------------------------------------ | ------ | ---------------------------------------------- |
| (unused)       | 9280f26c-a56f-43ea-b769-d5d732e1ac67 | 0x10   | Not used for Service 3 (used by Service 4)    |
| Device Info    | dc272a22-43f2-416b-8fa5-63a071542fac | 0x11   | **READ/WRITE**: Device info + text commands   |
| PIN            | d587c47f-ac6e-4388-a31c-e6cd380ba043 | 0x15   | Static PIN value `0x3412` (read-only)          |

**Important:** Despite sharing UUIDs with Service 4, Service 3 commands (getVer, powerOff, chargeCtrl) must be written to the **Device Info characteristic (dc272a22)**, NOT the Command characteristic. Responses also come via notification on dc272a22.

### Service 4: BLE API (0b9676ee-8352-440a-bf80-61541d578fcf)

REST-like API using binary envelope protocol.

| Characteristic | UUID                                 | Handle | Description                |
| -------------- | ------------------------------------ | ------ | -------------------------- |
| API Request    | 9280f26c-a56f-43ea-b769-d5d732e1ac67 | 0x10   | Write requests             |
| API Response   | d587c47f-ac6e-4388-a31c-e6cd380ba043 | 0x15   | **API responses (notify)** |

**Important:** Subscribe to `D587C47F` for API responses, NOT `DC272A22`.

---

## Service 3: Direct GATT Commands

Service 3 provides a simple text-based command interface. Write plain text command strings to the **Device Info characteristic (dc272a22)** and receive responses via GATT notification on the same characteristic.

**Note:** The firmware callback `ui_gatt_service_factory_cb` handles both READ and WRITE operations on dc272a22.

### Reading Device Info (dc272a22 characteristic)

Reading this characteristic returns a JSON string with device information.

**Response Format:**

```json
{
  "id": "DEADBEEFCAFE",
  "fwv": "1.1.3",
  "apiVersion": "1.0",
  "voltage": "3913",
  "level": "68"
}
```

| Field      | Type   | Description                            |
| ---------- | ------ | -------------------------------------- |
| id         | string | Device BLE MAC address (uppercase hex) |
| fwv        | string | Firmware version (major.minor.patch)   |
| apiVersion | string | API version, always "1.0"              |
| voltage    | string | Battery voltage in millivolts          |
| level      | string | Battery level percentage (0-100)       |

### Command: getVer

Returns device information (same as reading the Device Info characteristic).

**Request:** `getVer`

**Response (via notification):**

```json
{
  "id": "DEADBEEFCAFE",
  "fwv": "1.1.3",
  "apiVersion": "1.0",
  "voltage": "3913",
  "level": "68"
}
```

### Command: powerOff

Powers off the device. No response is sent (device shuts down).

**Request:** `powerOff`

**Response:** None (device powers off after 1 second delay)

**Sequence:**

1. Write "powerOff" to Device Info characteristic (dc272a22)
2. Device logs "Power off" message
3. Device calls `mcu_device_set_power()` after 1 second delay
4. Connection is lost as device shuts down

### Command: chargeCtrl

Controls battery charging behavior.

**Request:** `chargeCtrl`

**Response (via notification):**

```json
{ "id": "<MAC_ADDRESS>", "ret": "ok" }
```

**Behavior:** Toggles between charging modes (normal/high current/disabled) depending on battery management IC configuration.

### PIN Characteristic (d587c47f)

**Read Value:** `0x3412` (2 bytes, little-endian)

**Purpose:** Static read-only value for app pairing verification. Provides NO actual security - any BLE client can read this value and there is no authentication check before executing commands.

---

## Service 4: REST-like API Protocol

### Binary Envelope Format

Messages use a device transport header followed by **modified binme** binary envelope sections.

**Note:** The device uses a modified binme protocol:

- Header section uses type `0x03` instead of standard `0x01`
- Header section is 9 bytes (vs standard 8), with single-byte length at byte 8

```
[Device Transport Header - 4 bytes]
  bytes 0-1: total message length (big-endian, includes this header)
  bytes 2-3: sequence number (matches request ID, big-endian)

[Header Section - 9 bytes + data] (device-specific format)
  byte 0: type (0x03 = device header type)
  byte 1: format (0x01 = FORMAT_JSON)
  byte 2: isCompressed (0x00 = none, 0x01 = zlib)
  byte 3: flags (0x01 for requests, 0x00 for responses)
  bytes 4-7: reserved (0x00 0x00 0x00 0x00)
  byte 8: length (single byte)
  bytes 9+: header data (zlib compressed for requests, may be raw JSON for responses)

[Body Section - 8 bytes + data] (standard binme format)
  byte 0: type (0x02 = TYPE_BODY)
  byte 1: format (0x01 = FORMAT_JSON, 0x02 = FORMAT_STRING, 0x03 = FORMAT_BINARY)
  byte 2: isCompressed (0x00 = none, 0x01 = zlib)
  byte 3: reserved (0x00)
  bytes 4-7: length (big-endian uint32)
  bytes 8+: body data
```

**Binme Constants:**

| Constant      | Value | Description         |
| ------------- | ----- | ------------------- |
| TYPE_HEADER   | 0x01  | Header section type |
| TYPE_BODY     | 0x02  | Body section type   |
| FORMAT_JSON   | 0x01  | JSON data format    |
| FORMAT_STRING | 0x02  | UTF-8 string format |
| FORMAT_BINARY | 0x03  | Raw binary format   |

### Compression Notes

- **Requests:** Header and body are zlib compressed
- **Responses:** May have compression byte = 0x01 but data is NOT compressed
  - Always check for zlib magic byte `0x78` before decompressing

**Zlib magic bytes:**

- `78 01` - no/low compression
- `78 5e` - fast compression
- `78 9c` - default compression
- `78 da` - best compression

### JSON Envelope Format

**Request Envelope:**

```json
{
  "type": "httpRequest",
  "id": "00000000-0000-0000-0000-000000000001",
  "timestamp": 1768449224138,
  "method": "GET",
  "path": "/api/version",
  "headers": {}
}
```

**Response Envelope:**

```json
{
  "type": "httpResponse",
  "id": "00000000-0000-0000-0000-000000000001",
  "timestamp": 1768449232872,
  "statusCode": 200,
  "headers": {}
}
```

The response body is in the **body section**, not in the JSON envelope.

| Field      | Type   | Description                                      |
| ---------- | ------ | ------------------------------------------------ |
| type       | string | `httpRequest` or `httpResponse`                  |
| id         | string | Request ID (incrementing counter in UUID format) |
| timestamp  | number | Unix timestamp in milliseconds                   |
| method     | string | HTTP method: `GET` or `POST` (request only)      |
| path       | string | API endpoint path (request only)                 |
| statusCode | number | HTTP status code (response only)                 |
| headers    | object | Always empty `{}`                                |

### ID and Sequence Number Format

The ID is a zero-padded incrementing hex counter in UUID format:

- `00000000-0000-0000-0000-000000000001`
- `00000000-0000-0000-0000-000000000002`

The **transport header bytes 2-3** contain the same sequence number (e.g., `00 05` for ID `...000000000005`).

### Status Codes

| Code | Meaning               |
| ---- | --------------------- |
| 200  | Success               |
| 304  | Not Modified          |
| 400  | Bad Request           |
| 404  | Not Found             |
| 413  | Data size mismatch    |
| 417  | Expectation Failed    |
| 500  | Internal Server Error |

### MAC Address Format

The device MAC is formatted as **12 lowercase hex characters without separators**.

Example: MAC `DE:AD:BE:EF:CA:FE` becomes `deadbeefcafe`

---

## API Endpoints

### Routing Overview

The API handler (`ble_api_command_handler`) processes requests in this order:

1. **Simple Version Endpoints** (no MAC required)
2. **MAC-Authenticated Endpoints** (require device MAC in path)
3. **Custom Registered Endpoints** (XSFP/DDM handlers)

All MAC-authenticated endpoints use the base path: `/api/1.0/{mac}/`

---

### Simple Version Endpoints

These endpoints do NOT require the device MAC address.

#### GET /api/version

#### GET /api/1.0/version

Returns firmware and API version info.

**Note:** Returns 404 on firmware versions 1.0.10 and 1.1.0.

**Response:**

```json
{ "fwv": "1.1.3", "apiVersion": "1.0" }
```

---

### Device Info Endpoints

#### GET /api/1.0/{mac}

Returns device info.

**Response:**

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

#### GET /api/1.0/{mac}/stats

Returns device statistics including battery level.

**Response:**

```json
{
  "battery": 71,
  "batteryV": 3.888,
  "isLowBattery": false,
  "uptime": 607849,
  "signalDbm": -55
}
```

#### GET /api/1.0/{mac}/settings

Returns device settings.

**Response:**

```json
{
  "ch": "release",
  "name": "uacc-sfp-wizard",
  "isLedEnabled": true,
  "isHwResetBlocked": false,
  "uwsType": "us",
  "intervals": { "intStats": 1000 },
  "homekitEnabled": false
}
```

#### GET /api/1.0/{mac}/bt

Returns Bluetooth parameters.

**Response:**

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

---

### Device Control Endpoints

#### POST /api/1.0/{mac}/reboot

Reboots the device. The BLE connection will drop during reboot.

**Request body:** None (empty)

**Response:** Status 200 (connection may drop before response)

#### POST /api/1.0/{mac}/name

Sets the device friendly name. Maximum 28 characters (stored in 29-byte buffer with null terminator).

**Request Body (JSON):**

```json
{"name":"<new_name>"}
```

**Response:** HTTP 200 on success, HTTP 304 if unchanged, HTTP 500 on error

**Storage:** Name is persisted in NVS under namespace "UI_BLE" with key "FRI_NAME".

---

### Firmware Update Endpoints

#### GET /api/1.0/{mac}/fw

Returns firmware status.

**Response:**

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

| Field           | Type   | Description                          |
| --------------- | ------ | ------------------------------------ |
| hwv             | int    | Hardware version                     |
| fwv             | string | Firmware version                     |
| isUPdating      | bool   | Update in progress                   |
| status          | string | "inProgress", "complete", or "error" |
| progressPercent | int    | Update progress (0-100)              |
| remainingTime   | int    | Estimated remaining time             |

#### POST /api/1.0/{mac}/fw/start

Starts a firmware update.

**Request Body:** `size=<firmware_size_bytes>`

**Response:** HTTP 200 on success, HTTP 400 if size invalid, HTTP 500 on error

#### POST /api/1.0/{mac}/fw/data

Sends firmware data chunk.

**Request Body:** Raw binary firmware data

**Response:** HTTP 200 on success, HTTP 400 on error

#### POST /api/1.0/{mac}/fw/abort

Aborts an in-progress firmware update.

**Response:** HTTP 200

---

### SIF (Support Info File?) Operations

The SIF protocol returns a **tar archive** containing device logs and module EEPROM snapshots.

| Method | Path                       | Description              |
| ------ | -------------------------- | ------------------------ |
| POST   | `/api/1.0/{mac}/sif/start` | Start SIF read operation |
| GET    | `/api/1.0/{mac}/sif/info/` | SIF operation status     |
| GET    | `/api/1.0/{mac}/sif/data/` | Read SIF data chunk      |
| POST   | `/api/1.0/{mac}/sif/abort` | Abort SIF operation      |

**Note:** The `/sif/info/` and `/sif/data/` paths require a trailing slash.

#### POST /api/1.0/{mac}/sif/start

Initiates an SIF read operation.

**Request body:** None (empty)

**Response:**

```json
{ "status": "ready", "offset": 0, "chunk": 1024, "size": 512 }
```

| Field  | Description                          |
| ------ | ------------------------------------ |
| status | "ready" when initialized             |
| offset | Starting offset (always 0)           |
| chunk  | Maximum chunk size for data requests |
| size   | Total size in bytes                  |

#### GET /api/1.0/{mac}/sif/data/

Reads a chunk of data.

**Request body:**

```json
{ "status": "continue", "offset": 0, "chunk": 512 }
```

**Response body:** Raw binary data (not JSON).

**Important:** Large responses are **fragmented across multiple BLE notifications**. Accumulate payloads until `total_received >= total_length`.

#### GET /api/1.0/{mac}/sif/info/

Returns current operation status.

**Response:**

```json
{ "status": "finished", "offset": 512 }
```

**Status values:** `ready`, `continue`, `inprogress`, `complete`, `finished`

#### SIF Read Flow

```
POST /sif/start           → {"status":"ready","offset":0,"chunk":1024,"size":512}
GET  /sif/data/ (0,512)   → [512 bytes of raw data]
GET  /sif/info/           → {"status":"complete","offset":512}
```

#### SIF Archive Contents

| File                 | Size    | Description                                             |
| -------------------- | ------- | ------------------------------------------------------- |
| `syslog`             | ~5-10KB | Device logs (clears on reboot)                          |
| `sfp_primary.bin`    | 512     | SFP read via device screen                              |
| `sfp_secondary.bin`  | 512     | SFP read via API                                        |
| `qsfp_primary.bin`   | 640     | QSFP read via device screen                             |
| `qsfp_secondary.bin` | 640     | QSFP read via API                                       |
| `{PartNumber}.bin`   | 512/640 | Module database entries (keyed by S/N, see note though) |

**Notes:**

- `primary` = read via device screen, `secondary` = read via API
- Named `{PartNumber}.bin` files persist across reboots (flash storage)
- Named `{PartNumber}.bin` files are in the tar without their full path from the device filesystem, which means duplicates can occur. untarring normally will result in duplicates overwriting the previously decompressed file. This appears to be caused by part numbers with spaces and slashes, e.g. `OEM AXB23-192-20/GR` is stored on device as `/fs/sfp/OEM/AXB23-192-20/GR.bin` then added to the support dump as `GR.bin`.
- Files filled with `0xff` indicate no module present
- EEPROM format follows SFF-8472 (SFP) or SFF-8636 (QSFP)

---

### XSFP (Extended SFP) Operations

The XSFP protocol provides direct read/write access to SFP module EEPROM. Unlike SIF which returns a tar archive, XSFP works with raw binary data and supports **writing** to module EEPROM.

All XSFP endpoints require the full MAC path: `/api/1.0/{mac}/xsfp/...`

| Method | Path                                 | Description               |
| ------ | ------------------------------------ | ------------------------- |
| GET    | `/api/1.0/{mac}/xsfp/module/details` | Get module details        |
| GET    | `/api/1.0/{mac}/xsfp/sync/start`     | Get snapshot info         |
| POST   | `/api/1.0/{mac}/xsfp/sync/start`     | Initialize write transfer |
| GET    | `/api/1.0/{mac}/xsfp/sync/data`      | Read snapshot data        |
| POST   | `/api/1.0/{mac}/xsfp/sync/data`      | Write snapshot data chunk |
| POST   | `/api/1.0/{mac}/xsfp/sync/cancel`    | Cancel transfer           |
| GET    | `/api/1.0/{mac}/xsfp/module/start`   | Start module read         |
| GET    | `/api/1.0/{mac}/xsfp/module/data`    | Read module data          |
| POST   | `/api/1.0/{mac}/xsfp/recover`        | Recovery operation        |

#### GET /api/1.0/{mac}/xsfp/module/details

Returns module info without reading full EEPROM. **Requires firmware 1.1.0+**

**Response:**

```json
{
  "partNumber": "SFP-10G-T",
  "rev": "02",
  "vendor": "CSY103P17791",
  "sn": "CSY103P17791",
  "type": "sfp",
  "compliance": "10G BASE-SR"
}
```

| Field      | Description              |
| ---------- | ------------------------ |
| partNumber | Module part number       |
| rev        | Revision                 |
| vendor     | Vendor name/ID           |
| sn         | Serial number            |
| type       | "sfp" or "qsfp" (1.1.1+) |
| compliance | Transceiver compliance   |

**Note:** Returns 417 if no module inserted.

#### GET /api/1.0/{mac}/xsfp/sync/start

Returns information about current snapshot buffer.

**Response:**

```json
{
  "partNumber": "AXB32-192-20/GR",
  "vendor": "CI2508081988",
  "sn": "CI2508081988",
  "type": "sfp",
  "chunk": 512,
  "size": 512
}
```

**Notes:**

- Returns 417 if no module inserted.
- Only works if a module is inserted - I assume this is so it can choose which of the SFP or QSFP snapshot buffers to work with but do not currently have any QSFP modules to test with.

#### Snapshot Sizes

| Size              | Module Type                  |
| ----------------- | ---------------------------- |
| 512 bytes (0x200) | SFP module (A0h + A2h pages) |
| 640 bytes (0x280) | QSFP module                  |

#### XSFP Write Flow

1. **POST `/xsfp/sync/start`** with `{"size": 512}`
2. **POST `/xsfp/sync/data`** with raw binary EEPROM data
3. Device validates and applies snapshot on completion
4. User must hit write on the LCD to actually write the snapshot to a module

#### XSFP Read Flow

1. **GET `/xsfp/sync/start`** or **GET `/xsfp/module/start`**
2. **GET `/xsfp/sync/data`** or **GET `/xsfp/module/data`**

#### POST /api/1.0/{mac}/xsfp/recover

Restores module EEPROM from saved "golden snapshot" in device database.

**Request body:**

```json
{ "sn": "SERIALNUMBER", "wavelength": 1310 }
```

| Field      | Required | Description                              |
| ---------- | -------- | ---------------------------------------- |
| sn         | Yes      | Serial number of module to recover       |
| wavelength | No       | Override wavelength in restored snapshot |

**Response:** 200 on success, 404 if golden snapshot not found

---

### DDM (Digital Diagnostic Monitoring) Endpoints

DDM endpoints provide real-time diagnostic data from SFP/QSFP transceivers (temperature, voltage, TX/RX power, laser bias).

All DDM endpoints require the full MAC path: `/api/1.0/{mac}/ddm/...`

| Method | Path                       | Description         |
| ------ | -------------------------- | ------------------- |
| GET    | `/api/1.0/{mac}/ddm/start` | Start DDM report    |
| GET    | `/api/1.0/{mac}/ddm/data`  | Get DDM report data |

**Notes:**

- Currently there does not appear to be a way to start the DDM Info collection via API, so the user must hit "DDM Info" on the screen, and can toggle the laser on and off. This endpoint just returns the data since the last DDM Info session started.
- The data has approximately 1 second granularity; much lower than the display on the device

---

## Module Database

The device maintains a persistent database of module snapshots on internal flash.

### Storage Implementation

- **Filesystem:** LittleFS on ESP32-S3 flash
- **Format:** Individual binary files per module
- **Key:** Module serial number
- **Filename:** Part number suffix (e.g., `SFP-10G-SR.bin`)
- **Slots per module:** 2 (one for screen read, one for API read)

### Storage Limits

- No hardcoded limit on number of modules
- Limited only by flash partition size
- Each entry uses 512-640 bytes plus filesystem overhead
- Typical partitions can store hundreds of snapshots

---

## SFP Password Database

The firmware contains an embedded password database to unlock vendor-locked SFP/QSFP modules.

### Database Format

Entry structure changed between firmware versions:

| Firmware     | Entry Size | Fields                                                                |
| ------------ | ---------- | --------------------------------------------------------------------- |
| 1.0.x, 1.1.0 | 20 bytes   | read_only, part_number\*, locked, password[4], flags[3], cable_length |
| 1.1.1+       | 16 bytes   | read_only, part_number\*, locked, password[4], flags[3]               |

**Entry Structure (1.1.1+ - 16 bytes):**

```
Offset  Size  Field
0x00    4     read_only (uint32) - Skip if non-zero
0x04    4     part_number (char*) - Pointer to string
0x08    1     locked (bool) - Module requires unlock
0x09    4     password[4] - 4-byte unlock password
0x0D    3     flags[3] - Writable pages bitmask
```

### Flags Field

`flags[0]` indicates which EEPROM pages can be written after unlock:

| Bit | Value | Page         | Description                    |
| --- | ----- | ------------ | ------------------------------ |
| 0   | 0x01  | A0h / Lower  | Basic identity page            |
| 1   | 0x02  | A2h / Upper1 | SFP diagnostic or QSFP upper 1 |
| 2   | 0x04  | Upper 2      | QSFP upper page 2              |
| 3   | 0x08  | Upper 3      | QSFP upper page 3 (thresholds) |

**Common values:** `0x03` = SFP (A0h + A2h), `0x0F` = Full QSFP

### Password Lookup Algorithm

**1.0.10:** First match by part number, fallback tries ALL unique passwords from entire database

**1.1.3:** Collects all matching entries, deduplicates by password, tries each until success

### Database Summary

| Firmware | Entries | Entry Size | Unique Passwords |
| -------- | ------- | ---------- | ---------------- |
| 1.0.10   | 54      | 20 bytes   | 5                |
| 1.1.0    | 54      | 20 bytes   | 5                |
| 1.1.1    | 58      | 16 bytes   | 6                |
| 1.1.3    | 59      | 16 bytes   | 6                |

### Known Passwords

| Password (Hex) | Password (ASCII) | Used By                              |
| -------------- | ---------------- | ------------------------------------ |
| `00 00 10 11`  | -                | Most AOC/Uplink/OM modules (default) |
| `78 56 34 12`  | -                | DAC-SFP28-3M, OM-SFP10-DWDM          |
| `53 46 50 58`  | "SFPX"           | OM-SFP28-LR                          |
| `80 81 82 83`  | -                | OM-QSFP28-LR4, OM-QSFP28-PSM4        |
| `51 53 46 50`  | "QSFP"           | OM-QSFP28-SR4                        |
| `63 73 77 77`  | "csww"           | Alternate for OM modules (1.1.1+)    |

### Extracting the Password Database

```bash
# Extract and display
sfpw fw passdb firmware.bin

# Output as JSON
sfpw fw passdb -j firmware.bin

# Evaluate specific part number
sfpw fw passdb -s "OM-SFP28-LR" firmware.bin
```

---

## Example Packets

### Request Packet

Raw request to `/api/1.0/deadbeefcafe/stats`:

```
Transport header:  00 9a 00 05
                   ^^^^^       - total length (154 bytes)
                         ^^^^^ - sequence number (5)

Header section:    03 01 01 01 00 00 00 00 7d
                   ^^ - type (0x03 = device header)
                      ^^ - format (0x01 = FORMAT_JSON)
                         ^^ - isCompressed (0x01 = zlib)
                            ^^ - flags (0x01 for requests)
                               ^^^^^^^^^^^ - reserved
                                           ^^ - length (125 bytes)
                   [125 bytes of zlib compressed JSON]

Body section:      02 01 01 00 00 00 00 08
                   ^^ - type (0x02 = TYPE_BODY)
                      ^^ - format (0x01 = FORMAT_JSON)
                         ^^ - isCompressed (0x01 = zlib)
                            ^^ - reserved
                               ^^^^^^^^^^^ - length (8 bytes)
                   78 9c 03 00 00 00 00 01   (zlib compressed empty body)
```

### Response Packet

Raw response from `/api/version`:

```
Transport header:  00 b2 00 01
                   ^^^^^       - total length (178 bytes)
                         ^^^^^ - sequence number (1)

Header section:    03 01 01 00 00 00 00 00 7b
                   ^^ - type (0x03 = device header)
                      ^^ - format (0x01 = FORMAT_JSON)
                         ^^ - isCompressed (0x01, but NOT actually compressed!)
                            ^^ - flags (0x00 for responses)
                               ^^^^^^^^^^^ - reserved
                                           ^^ - length (123 bytes)
                   [123 bytes of RAW JSON - not compressed despite flag]

Body section:      02 01 00 00 00 00 00 22
                   ^^ - type (0x02 = TYPE_BODY)
                      ^^ - format (0x01 = FORMAT_JSON)
                         ^^ - isCompressed (0x00 = none)
                            ^^ - reserved
                               ^^^^^^^^^^^ - length (34 bytes)
                   {"fwv":"1.1.1","apiVersion":"1.0"}
```

---

## Error Handling

Service 3 commands that fail will either:

1. Return no response (device crashed or powered off)
2. Log an error message (visible in device debug output)

Unknown commands logged as: `E (%lu) BLE_GATT: Unknown command: %s`

Service 4 API errors return appropriate HTTP status codes with optional error message in body.
