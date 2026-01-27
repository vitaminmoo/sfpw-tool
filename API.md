# UACC SFP Wizard BLE API Protocol

> [!WARNING]
> Maintained via LLM with human guidance - certainly contains errors

This document describes the BLE API protocol for the UACC SFP Wizard device.

---

## Quick Reference

### Device Management

| Method | Path | Description |
|--------|------|-------------|
| GET | [`/api/1.0/{mac}`](#get-api10mac) | Device info (type, firmware, IDs) |
| GET | [`/api/1.0/{mac}/stats`](#get-api10macstats) | Battery, uptime, signal strength |
| GET | [`/api/1.0/{mac}/settings`](#get-api10macsettings) | Device configuration |
| GET | [`/api/1.0/{mac}/bt`](#get-api10macbt) | Bluetooth parameters |
| POST | [`/api/1.0/{mac}/reboot`](#post-api10macreboot) | Reboot device |
| POST | [`/api/1.0/{mac}/name`](#post-api10macname) | Set device name |

### Module Operations (XSFP)

| Method | Path | Description |
|--------|------|-------------|
| GET | [`/api/1.0/{mac}/xsfp/module/details`](#get-api10macxsfpmoduledetails) | Quick module info (1.1.0+) |
| GET | [`/api/1.0/{mac}/xsfp/sync/start`](#get-api10macxsfpsyncstart) | Snapshot buffer info |
| POST | [`/api/1.0/{mac}/xsfp/sync/start`](#post-api10macxsfpsyncstart) | Initialize write transfer |
| GET | [`/api/1.0/{mac}/xsfp/sync/data`](#get-api10macxsfpsyncdata) | Read snapshot data |
| POST | [`/api/1.0/{mac}/xsfp/sync/data`](#post-api10macxsfpsyncdata) | Write snapshot data |
| POST | [`/api/1.0/{mac}/xsfp/sync/cancel`](#post-api10macxsfpsynccancel) | Cancel transfer |
| GET | [`/api/1.0/{mac}/xsfp/module/start`](#get-api10macxsfpmodulestart) | Start live module read |
| GET | [`/api/1.0/{mac}/xsfp/module/data`](#get-api10macxsfpmoduledata) | Read live module data |
| POST | [`/api/1.0/{mac}/xsfp/recover`](#post-api10macxsfprecover) | Restore from golden snapshot |

### DDM (Digital Diagnostics)

| Method | Path | Description |
|--------|------|-------------|
| GET | [`/api/1.0/{mac}/ddm/start`](#get-api10macddmstart) | Start DDM report |
| GET | [`/api/1.0/{mac}/ddm/data`](#get-api10macddmdata) | Get DDM data |

### Support/Debug (SIF)

| Method | Path | Description |
|--------|------|-------------|
| POST | [`/api/1.0/{mac}/sif/start`](#post-api10macsifstart) | Start support dump |
| GET | [`/api/1.0/{mac}/sif/info/`](#get-api10macsifinfo) | Dump status |
| GET | [`/api/1.0/{mac}/sif/data/`](#get-api10macsifdata) | Read dump data |
| POST | [`/api/1.0/{mac}/sif/abort`](#post-api10macsifabort) | Abort dump |

### Firmware

| Method | Path | Description |
|--------|------|-------------|
| GET | [`/api/1.0/{mac}/fw`](#get-api10macfw) | Firmware status |
| POST | [`/api/1.0/{mac}/fw/start`](#post-api10macfwstart) | Start firmware update |
| POST | [`/api/1.0/{mac}/fw/data`](#post-api10macfwdata) | Send firmware chunk |
| POST | [`/api/1.0/{mac}/fw/abort`](#post-api10macfwabort) | Abort update |

### Simple Endpoints (no MAC required)

| Method | Path | Description |
|--------|------|-------------|
| GET | [`/api/version`](#get-apiversion) | API version |
| GET | [`/api/1.0/version`](#get-api10version) | API version (alternate) |

---

## Firmware Version Compatibility

Tested firmware versions: **1.0.10**, **1.1.0**, **1.1.1**, **1.1.3**

| Endpoint | 1.0.10 | 1.1.0 | 1.1.1+ | Notes |
|----------|:------:|:-----:|:------:|-------|
| [`/api/1.0/{mac}`](#get-api10mac) | :white_check_mark: | :white_check_mark: | :white_check_mark: | Device info |
| [`/api/1.0/{mac}/stats`](#get-api10macstats) | :white_check_mark: | :white_check_mark: | :white_check_mark: | |
| [`/api/1.0/{mac}/settings`](#get-api10macsettings) | :white_check_mark: | :white_check_mark: | :white_check_mark: | |
| [`/api/1.0/{mac}/bt`](#get-api10macbt) | :white_check_mark: | :white_check_mark: | :white_check_mark: | |
| [`/api/1.0/{mac}/fw`](#get-api10macfw) | :white_check_mark: | :white_check_mark: | :white_check_mark: | |
| [`/api/1.0/{mac}/xsfp/module/details`](#get-api10macxsfpmoduledetails) | :x: | :white_check_mark: | :white_check_mark: | Added in 1.1.0, `type` field in 1.1.1 |
| [`/api/1.0/{mac}/xsfp/sync/start`](#get-api10macxsfpsyncstart) | :white_check_mark: | :white_check_mark: | :white_check_mark: | Returns 417 if no module; `type` field in 1.1.1 |
| [`/api/1.0/{mac}/xsfp/module/start`](#get-api10macxsfpmodulestart) | :grey_question: | :white_check_mark: | :white_check_mark: | Untested on 1.0.10 |
| [`/api/1.0/{mac}/sif/*`](#sif-support-dump-operations) | :white_check_mark: | :white_check_mark: | :white_check_mark: | SIF archive operations |

**Key changes:**
- **1.1.0:** Added `/xsfp/module/details` for quick module info without full EEPROM read
- **1.1.1:** Added `type` field ("sfp" or "qsfp") to module detail responses

---

## Protocol Overview

Communication uses a REST-like API over BLE GATT. Requests and responses are wrapped in a binary envelope format.

```
┌─────────────────────────────────────────────────────────────────────┐
│                         BLE Communication                           │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   ┌──────────┐         Write to 9280F26C          ┌──────────┐      │
│   │          │ ──────────────────────────────────>│          │      │
│   │  Client  │                                    │  Device  │      │
│   │          │<────────────────────────────────── │          │      │
│   └──────────┘       Notify on D587C47F           └──────────┘      │
│                                                                     │
│   Request:  [Transport Header][Header Section][Body Section]        │
│   Response: [Transport Header][Header Section][Body Section]        │
│                      ^              ^              ^                │
│                      │              │              │                │
│                   4 bytes      9+ bytes        8+ bytes             │
│                   big-endian   zlib JSON       raw/zlib             │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### MAC Address Format

The device MAC is formatted as **12 lowercase hex characters without separators**.

Example: `DE:AD:BE:EF:CA:FE` → `deadbeefcafe`

### Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 304 | Not Modified |
| 400 | Bad Request |
| 404 | Not Found |
| 413 | Data size mismatch |
| 417 | Expectation Failed (e.g., no module inserted) |
| 500 | Internal Server Error |

---

## Binary Envelope Format

Messages use a device transport header followed by **modified binme** binary envelope sections.

> [!NOTE]
> The device uses a modified binme protocol:
> - Header section uses type `0x03` instead of standard `0x01`
> - Header section is 9 bytes (vs standard 8), with single-byte length at byte 8

### Message Structure

```
┌─────────────────────────────────────────────────────────────────┐
│ Transport Header (4 bytes)                                      │
├──────────────┬──────────────────────────────────────────────────┤
│ Bytes 0-1    │ Total message length (big-endian, includes header)│
│ Bytes 2-3    │ Sequence number (big-endian, matches request ID) │
└──────────────┴──────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ Header Section (9 bytes + data) — device-specific format        │
├──────────────┬──────────────────────────────────────────────────┤
│ Byte 0       │ Type: 0x03 (device header type)                  │
│ Byte 1       │ Format: 0x01 (JSON)                              │
│ Byte 2       │ Compressed: 0x00 (none) or 0x01 (zlib)           │
│ Byte 3       │ Flags: 0x01 (request) or 0x00 (response)         │
│ Bytes 4-7    │ Reserved: 0x00 0x00 0x00 0x00                    │
│ Byte 8       │ Length (single byte)                             │
│ Bytes 9+     │ Header data (JSON, possibly zlib compressed)     │
└──────────────┴──────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ Body Section (8 bytes + data) — standard binme format           │
├──────────────┬──────────────────────────────────────────────────┤
│ Byte 0       │ Type: 0x02 (body)                                │
│ Byte 1       │ Format: 0x01 (JSON), 0x02 (string), 0x03 (binary)│
│ Byte 2       │ Compressed: 0x00 (none) or 0x01 (zlib)           │
│ Byte 3       │ Reserved: 0x00                                   │
│ Bytes 4-7    │ Length (big-endian uint32)                       │
│ Bytes 8+     │ Body data                                        │
└──────────────┴──────────────────────────────────────────────────┘
```

### Binme Constants

| Constant | Value | Description |
|----------|-------|-------------|
| TYPE_HEADER | 0x01 | Standard header section type |
| TYPE_BODY | 0x02 | Body section type |
| FORMAT_JSON | 0x01 | JSON data format |
| FORMAT_STRING | 0x02 | UTF-8 string format |
| FORMAT_BINARY | 0x03 | Raw binary format |

### JSON Envelope

**Request:**
```json
{
  "type": "httpRequest",
  "id": "00000000-0000-0000-0000-000000000001",
  "timestamp": 1768449224138,
  "method": "GET",
  "path": "/api/1.0/deadbeefcafe/stats",
  "headers": {}
}
```

**Response:**
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

| Field | Type | Description |
|-------|------|-------------|
| type | string | `httpRequest` or `httpResponse` |
| id | string | Request ID (incrementing counter in UUID format) |
| timestamp | number | Unix timestamp in milliseconds |
| method | string | HTTP method: `GET` or `POST` (request only) |
| path | string | API endpoint path (request only) |
| statusCode | number | HTTP status code (response only) |
| headers | object | Always empty `{}` |

### ID and Sequence Number

The ID is a zero-padded incrementing hex counter in UUID format:
- `00000000-0000-0000-0000-000000000001`
- `00000000-0000-0000-0000-000000000002`

The **transport header bytes 2-3** contain the same sequence number (e.g., `00 05` for ID ending in `...000005`).

### Compression

> [!IMPORTANT]
> **Requests:** Header and body are zlib compressed
> **Responses:** May have compression byte = 0x01 but data is NOT compressed. Always check for zlib magic byte `0x78` before decompressing.

**Zlib magic bytes:**
| Bytes | Compression Level |
|-------|-------------------|
| `78 01` | None/low |
| `78 5e` | Fast |
| `78 9c` | Default |
| `78 da` | Best |

---

## Example Packets

<details>
<summary><strong>Request Packet</strong> — GET /api/1.0/deadbeefcafe/stats</summary>

```
Transport header:  00 9a 00 05
                   ├───┘ ├───┘
                   │     └─── sequence number (5)
                   └───────── total length (154 bytes)

Header section:    03 01 01 01 00 00 00 00 7d [125 bytes zlib JSON]
                   │  │  │  │  └──────────┘ └─ length (125 bytes)
                   │  │  │  └─ flags (0x01 = request)
                   │  │  └──── compressed (0x01 = zlib)
                   │  └─────── format (0x01 = JSON)
                   └────────── type (0x03 = device header)

Body section:      02 01 01 00 00 00 00 08 78 9c 03 00 00 00 00 01
                   │  │  │  │  └──────────┘ └─────────────────────┘
                   │  │  │  │       │              └─ zlib compressed empty body
                   │  │  │  │       └─ length (8 bytes)
                   │  │  │  └─ reserved
                   │  │  └──── compressed (0x01 = zlib)
                   │  └─────── format (0x01 = JSON)
                   └────────── type (0x02 = body)
```

</details>

<details>
<summary><strong>Response Packet</strong> — GET /api/version</summary>

```
Transport header:  00 b2 00 01
                   ├───┘ ├───┘
                   │     └─── sequence number (1)
                   └───────── total length (178 bytes)

Header section:    03 01 01 00 00 00 00 00 7b [123 bytes RAW JSON]
                   │  │  │  │  └──────────┘ └─ length (123 bytes)
                   │  │  │  └─ flags (0x00 = response)
                   │  │  └──── compressed (0x01, but NOT actually compressed!)
                   │  └─────── format (0x01 = JSON)
                   └────────── type (0x03 = device header)

Body section:      02 01 00 00 00 00 00 22 {"fwv":"1.1.1","apiVersion":"1.0"}
                   │  │  │  │  └──────────┘ └──────────────────────────────────┘
                   │  │  │  │       │              └─ raw JSON body (34 bytes)
                   │  │  │  │       └─ length (34 bytes)
                   │  │  │  └─ reserved
                   │  │  └──── compressed (0x00 = none)
                   │  └─────── format (0x01 = JSON)
                   └────────── type (0x02 = body)
```

</details>

---

## BLE Services Overview

The device exposes four BLE services:

### Service 1: Generic Access (0x1800) — Standard

| Characteristic | UUID | Description |
|----------------|------|-------------|
| Device Name | 0x2A00 | Returns "UACC-SFP-Wizard" |
| Appearance | 0x2A01 | Returns 0x0000 (Generic) |

### Service 2: Generic Attribute (0x1801) — Standard

| Characteristic | UUID | Description |
|----------------|------|-------------|
| Service Changed | 0x2A05 | Standard GATT notification |
| Client Supported Features | 0x2B3A | Client features |
| Database Hash | 0x2B29 | GATT database hash |

### Service 3: Device Info & Control (8e60f02e-f699-4865-b83f-f40501752184)

Simple text-based command interface for device control.

| Characteristic | UUID | Handle | Description |
|----------------|------|--------|-------------|
| (unused) | 9280f26c-a56f-43ea-b769-d5d732e1ac67 | 0x10 | Not used for Service 3 |
| Device Info | dc272a22-43f2-416b-8fa5-63a071542fac | 0x11 | **READ/WRITE**: Device info + text commands |
| PIN | d587c47f-ac6e-4388-a31c-e6cd380ba043 | 0x15 | Static PIN `0x3412` (read-only) |

> [!IMPORTANT]
> Despite sharing UUIDs with Service 4, Service 3 commands (`getVer`, `powerOff`, `chargeCtrl`) must be written to the **Device Info characteristic (dc272a22)**, NOT the Command characteristic. Responses also come via notification on dc272a22.

### Service 4: BLE API (0b9676ee-8352-440a-bf80-61541d578fcf)

REST-like API using binary envelope protocol.

| Characteristic | UUID | Handle | Description |
|----------------|------|--------|-------------|
| API Request | 9280f26c-a56f-43ea-b769-d5d732e1ac67 | 0x10 | Write requests |
| API Response | d587c47f-ac6e-4388-a31c-e6cd380ba043 | 0x15 | **Subscribe for responses** |

> [!IMPORTANT]
> Subscribe to `D587C47F` for API responses, NOT `DC272A22`.

---

## Service 3: Direct GATT Commands

Service 3 provides a simple text-based command interface. Write plain text to the **Device Info characteristic (dc272a22)** and receive responses via GATT notification on the same characteristic.

### Reading Device Info

Reading the dc272a22 characteristic returns device information:

```json
{
  "id": "DEADBEEFCAFE",
  "fwv": "1.1.3",
  "apiVersion": "1.0",
  "voltage": "3913",
  "level": "68"
}
```

| Field | Type | Description |
|-------|------|-------------|
| id | string | Device BLE MAC address (uppercase hex) |
| fwv | string | Firmware version (major.minor.patch) |
| apiVersion | string | API version, always "1.0" |
| voltage | string | Battery voltage in millivolts |
| level | string | Battery level percentage (0-100) |

### Command: getVer

Returns device information (same as reading the characteristic).

| | |
|---|---|
| **Request** | `getVer` |
| **Response** | JSON (same format as reading characteristic) |

### Command: powerOff

Powers off the device.

| | |
|---|---|
| **Request** | `powerOff` |
| **Response** | None (device shuts down after 1 second) |

### Command: chargeCtrl

Controls battery charging behavior.

| | |
|---|---|
| **Request** | `chargeCtrl` |
| **Response** | `{"id": "<MAC>", "ret": "ok"}` |

Toggles between charging modes (normal/high current/disabled) depending on battery management IC configuration.

### PIN Characteristic

| | |
|---|---|
| **Read Value** | `0x3412` (2 bytes, little-endian) |
| **Purpose** | Static value for app pairing verification |

> [!NOTE]
> Provides NO actual security - any BLE client can read this value with no authentication.

---

## API Endpoints

### Routing

The API handler processes requests in order:
1. **Simple Version Endpoints** (no MAC required)
2. **MAC-Authenticated Endpoints** (require device MAC in path)
3. **Custom Registered Endpoints** (XSFP/DDM handlers)

All MAC-authenticated endpoints use base path: `/api/1.0/{mac}/`

---

### Simple Endpoints

#### GET /api/version

#### GET /api/1.0/version

Returns firmware and API version info.

> [!NOTE]
> Returns 404 on firmware versions 1.0.10 and 1.1.0.

**Response:**
```json
{"fwv": "1.1.3", "apiVersion": "1.0"}
```

---

### Device Management

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

Returns device statistics.

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
  "intervals": {"intStats": 1000},
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

#### POST /api/1.0/{mac}/reboot

Reboots the device. BLE connection will drop during reboot.

| | |
|---|---|
| **Request Body** | None |
| **Response** | 200 (connection may drop before response) |

#### POST /api/1.0/{mac}/name

Sets the device friendly name.

| | |
|---|---|
| **Request Body** | `{"name": "<new_name>"}` |
| **Max Length** | 28 characters |
| **Response** | 200 success, 304 unchanged, 500 error |
| **Storage** | NVS namespace "UI_BLE", key "FRI_NAME" |

---

### Module Operations (XSFP)

The XSFP protocol provides direct read/write access to SFP/QSFP module EEPROM.

#### Snapshot Sizes

| Size | Module Type |
|------|-------------|
| 512 bytes (0x200) | SFP (A0h + A2h pages) |
| 640 bytes (0x280) | QSFP |

#### GET /api/1.0/{mac}/xsfp/module/details

Returns module info without reading full EEPROM.

> [!NOTE]
> Requires firmware 1.1.0+. Returns 417 if no module inserted.

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

| Field | Description |
|-------|-------------|
| partNumber | Module part number |
| rev | Revision |
| vendor | Vendor name/ID |
| sn | Serial number |
| type | "sfp" or "qsfp" (1.1.1+) |
| compliance | Transceiver compliance |

#### GET /api/1.0/{mac}/xsfp/sync/start

Returns information about current snapshot buffer.

> [!NOTE]
> Returns 417 if no module inserted. Only works with module present (selects SFP vs QSFP buffer).

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

#### POST /api/1.0/{mac}/xsfp/sync/start

Initialize a write transfer to the snapshot buffer.

| | |
|---|---|
| **Request Body** | `{"size": 512}` |
| **Response** | 200 on success |

#### GET /api/1.0/{mac}/xsfp/sync/data

Read snapshot data from the buffer.

**Response Body:** Raw binary EEPROM data

#### POST /api/1.0/{mac}/xsfp/sync/data

Write snapshot data chunk.

| | |
|---|---|
| **Request Body** | Raw binary EEPROM data |
| **Response** | 200 on success |

#### POST /api/1.0/{mac}/xsfp/sync/cancel

Cancel current transfer.

#### GET /api/1.0/{mac}/xsfp/module/start

Start a live module read.

#### GET /api/1.0/{mac}/xsfp/module/data

Read live module data.

**Response Body:** Raw binary EEPROM data

#### POST /api/1.0/{mac}/xsfp/recover

Restore module EEPROM from saved "golden snapshot" in device database.

**Request Body:**
```json
{"sn": "SERIALNUMBER", "wavelength": 1310}
```

| Field | Required | Description |
|-------|----------|-------------|
| sn | Yes | Serial number of module to recover |
| wavelength | No | Override wavelength in restored snapshot |

**Response:** 200 on success, 404 if golden snapshot not found

#### XSFP Read Flow

```
GET  /xsfp/sync/start       → {"partNumber":"...", "size":512, ...}
GET  /xsfp/sync/data        → [512 bytes raw EEPROM data]
```

#### XSFP Write Flow

```
POST /xsfp/sync/start       {"size": 512}
POST /xsfp/sync/data        [512 bytes raw EEPROM data]
                            → Device validates and stores snapshot
                            → User must press "Write" on LCD to flash module
```

---

### DDM (Digital Diagnostic Monitoring)

DDM endpoints provide real-time diagnostic data from SFP/QSFP transceivers (temperature, voltage, TX/RX power, laser bias).

> [!NOTE]
> Currently there is no way to start DDM collection via API. User must press "DDM Info" on device screen. These endpoints return data from the last DDM session. Data has ~1 second granularity (lower than device display).

#### GET /api/1.0/{mac}/ddm/start

Start DDM report.

#### GET /api/1.0/{mac}/ddm/data

Get DDM report data.

---

### SIF (Support Dump) Operations

The SIF protocol returns a **tar archive** containing device logs and module EEPROM snapshots.

> [!NOTE]
> The `/sif/info/` and `/sif/data/` paths require a **trailing slash**.

#### POST /api/1.0/{mac}/sif/start

Initiates a SIF read operation.

| | |
|---|---|
| **Request Body** | None |

**Response:**
```json
{"status": "ready", "offset": 0, "chunk": 1024, "size": 512}
```

| Field | Description |
|-------|-------------|
| status | "ready" when initialized |
| offset | Starting offset (always 0) |
| chunk | Maximum chunk size for data requests |
| size | Total archive size in bytes |

#### GET /api/1.0/{mac}/sif/info/

Returns current operation status.

**Response:**
```json
{"status": "finished", "offset": 512}
```

**Status values:** `ready`, `continue`, `inprogress`, `complete`, `finished`

#### GET /api/1.0/{mac}/sif/data/

Reads a chunk of archive data.

**Request Body:**
```json
{"status": "continue", "offset": 0, "chunk": 512}
```

**Response Body:** Raw binary data (not JSON)

> [!IMPORTANT]
> Large responses are **fragmented across multiple BLE notifications**. Accumulate payloads until `total_received >= total_length`.

#### POST /api/1.0/{mac}/sif/abort

Abort SIF operation.

#### SIF Read Flow

```
POST /sif/start           → {"status":"ready","offset":0,"chunk":1024,"size":512}
GET  /sif/data/ {0,512}   → [512 bytes raw tar data]
GET  /sif/info/           → {"status":"complete","offset":512}
```

#### SIF Archive Contents

| File | Size | Description |
|------|------|-------------|
| `syslog` | ~5-10KB | Device logs (clears on reboot) |
| `sfp_primary.bin` | 512 | SFP read via device screen |
| `sfp_secondary.bin` | 512 | SFP read via API |
| `qsfp_primary.bin` | 640 | QSFP read via device screen |
| `qsfp_secondary.bin` | 640 | QSFP read via API |
| `{PartNumber}.bin` | 512/640 | Module database entries |

> [!WARNING]
> Named `{PartNumber}.bin` files are added to tar without their full device path. Part numbers with spaces/slashes (e.g., `OEM AXB23-192-20/GR` stored as `/fs/sfp/OEM/AXB23-192-20/GR.bin`) become `GR.bin` in the archive. Duplicate filenames will overwrite when extracting.

**Notes:**
- `primary` = read via device screen, `secondary` = read via API
- Named files persist across reboots (flash storage)
- Files filled with `0xff` indicate no module was present
- EEPROM format follows SFF-8472 (SFP) or SFF-8636 (QSFP)

---

### Firmware Updates

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

| Field | Type | Description |
|-------|------|-------------|
| hwv | int | Hardware version |
| fwv | string | Firmware version |
| isUPdating | bool | Update in progress |
| status | string | "inProgress", "complete", or "error" |
| progressPercent | int | Progress (0-100) |
| remainingTime | int | Estimated remaining time |

#### POST /api/1.0/{mac}/fw/start

Starts a firmware update.

| | |
|---|---|
| **Request Body** | `size=<firmware_size_bytes>` |
| **Response** | 200 success, 400 invalid size, 500 error |

#### POST /api/1.0/{mac}/fw/data

Sends firmware data chunk.

| | |
|---|---|
| **Request Body** | Raw binary firmware data |
| **Response** | 200 success, 400 error |

#### POST /api/1.0/{mac}/fw/abort

Aborts an in-progress firmware update.

---

## Module Database

The device maintains a persistent database of module snapshots on internal flash.

### Storage Details

| Property | Value |
|----------|-------|
| Filesystem | LittleFS on ESP32-S3 flash |
| Format | Individual binary files per module |
| Key | Module serial number |
| Filename | Part number suffix (e.g., `SFP-10G-SR.bin`) |
| Slots per module | 2 (screen read + API read) |

### Limits

- No hardcoded limit on number of modules
- Limited only by flash partition size
- Each entry uses 512-640 bytes plus filesystem overhead
- Typical partitions can store hundreds of snapshots

---

## SFP Password Database

<details>
<summary><strong>Password Database Details</strong></summary>

The firmware contains an embedded password database to unlock vendor-locked SFP/QSFP modules.

### Database Format

Entry structure changed between firmware versions:

| Firmware | Entry Size | Fields |
|----------|------------|--------|
| 1.0.x, 1.1.0 | 20 bytes | read_only, part_number*, locked, password[4], flags[3], cable_length |
| 1.1.1+ | 16 bytes | read_only, part_number*, locked, password[4], flags[3] |

**Entry Structure (1.1.1+ — 16 bytes):**

```
Offset  Size  Field
0x00    4     read_only (uint32) — Skip if non-zero
0x04    4     part_number (char*) — Pointer to string
0x08    1     locked (bool) — Module requires unlock
0x09    4     password[4] — 4-byte unlock password
0x0D    3     flags[3] — Writable pages bitmask
```

### Flags Field

`flags[0]` indicates which EEPROM pages can be written after unlock:

| Bit | Value | Page | Description |
|-----|-------|------|-------------|
| 0 | 0x01 | A0h / Lower | Basic identity page |
| 1 | 0x02 | A2h / Upper1 | SFP diagnostic or QSFP upper 1 |
| 2 | 0x04 | Upper 2 | QSFP upper page 2 |
| 3 | 0x08 | Upper 3 | QSFP upper page 3 (thresholds) |

**Common values:** `0x03` = SFP (A0h + A2h), `0x0F` = Full QSFP

### Password Lookup Algorithm

- **1.0.10:** First match by part number, fallback tries ALL unique passwords from entire database
- **1.1.3:** Collects all matching entries, deduplicates by password, tries each until success

### Database Summary

| Firmware | Entries | Entry Size | Unique Passwords |
|----------|---------|------------|------------------|
| 1.0.10 | 54 | 20 bytes | 5 |
| 1.1.0 | 54 | 20 bytes | 5 |
| 1.1.1 | 58 | 16 bytes | 6 |
| 1.1.3 | 59 | 16 bytes | 6 |

### Known Passwords

| Password (Hex) | Password (ASCII) | Used By |
|----------------|------------------|---------|
| `00 00 10 11` | — | Most AOC/Uplink/OM modules (default) |
| `78 56 34 12` | — | DAC-SFP28-3M, OM-SFP10-DWDM |
| `53 46 50 58` | "SFPX" | OM-SFP28-LR |
| `80 81 82 83` | — | OM-QSFP28-LR4, OM-QSFP28-PSM4 |
| `51 53 46 50` | "QSFP" | OM-QSFP28-SR4 |
| `63 73 77 77` | "csww" | Alternate for OM modules (1.1.1+) |

### Extracting the Password Database

```bash
# Extract and display
sfpw fw passdb firmware.bin

# Output as JSON
sfpw fw passdb -j firmware.bin

# Evaluate specific part number
sfpw fw passdb -s "OM-SFP28-LR" firmware.bin
```

</details>

---

## Error Handling

**Service 3 commands** that fail will either:
1. Return no response (device crashed or powered off)
2. Log an error message (visible in device debug output)

Unknown commands logged as: `E (%lu) BLE_GATT: Unknown command: %s`

**Service 4 API errors** return appropriate HTTP status codes with optional error message in body.
