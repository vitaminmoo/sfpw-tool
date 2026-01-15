# UACC SFP Wizard BLE API Protocol

This document describes the BLE API protocol for the UACC SFP Wizard device (firmware version 1.1.1).

## Protocol Overview

The protocol uses **JSON messages wrapped in a binary envelope over BLE GATT**, with **zlib compression** for requests.

### BLE Characteristics

| Service UUID | Characteristic UUID | Handle | Purpose |
|-------------|---------------------|--------|---------|
| 8E60F02E-F699-4865-B83F-F40501752184 | 9280F26C-A56F-43EA-B769-D5D732E1AC67 | 0x10 | Write requests |
| 8E60F02E-F699-4865-B83F-F40501752184 | DC272A22-43F2-416B-8FA5-63A071542FAC | 0x11 | Device info (read) |
| 8E60F02E-F699-4865-B83F-F40501752184 | D587C47F-AC6E-4388-A31C-E6CD380BA043 | 0x15 | **API responses (notify)** |

**Important:** Subscribe to `D587C47F` for API responses, NOT `DC272A22`.

### Binary Envelope Format

Messages use a binary envelope with an outer header, header section (JSON envelope), and body section.

```
[Outer Header - 4 bytes]
  bytes 0-1: total message length (big-endian)
  bytes 2-3: sequence number (matches request ID, big-endian)

[Header Section - 9 bytes + data]
  byte 0: marker (0x03 = header section)
  byte 1: format (0x01 = JSON)
  byte 2: compression (0x01 = zlib for requests, may be 0x01 but uncompressed for responses)
  byte 3: flags (0x01 for requests, 0x00 for responses)
  bytes 4-7: reserved (0x00 0x00 0x00 0x00)
  byte 8: data length (single byte)
  bytes 9+: header data (zlib compressed for requests, raw JSON for responses)

[Body Section - 8 bytes + data]
  byte 0: marker (0x02 = body section)
  byte 1: format (0x01 = JSON)
  byte 2: compression (0x01 = zlib, 0x00 = none)
  byte 3: reserved (0x00)
  bytes 4-7: data length (big-endian)
  bytes 8+: body data
```

### Compression Notes

- **Requests:** Header and body are zlib compressed
- **Responses:** May have compression byte = 0x01 but data is NOT compressed
  - Always check for zlib magic byte `0x78` before decompressing

**Zlib magic bytes** (first byte is always `0x78`, second varies by compression level):
- `78 01` - no/low compression
- `78 5e` - fast compression
- `78 9c` - default compression
- `78 da` - best compression

---

## JSON Envelope Format

### Request Envelope
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

### Response Envelope
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

### Key Fields

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `httpRequest` or `httpResponse` |
| `id` | string | Request ID for correlation (incrementing counter in UUID format) |
| `timestamp` | number | Unix timestamp in milliseconds |
| `method` | string | HTTP method: `GET` or `POST` (request only) |
| `path` | string | API endpoint path (request only) |
| `statusCode` | number | HTTP status code (response only) |
| `headers` | object | Always empty `{}` |

### ID and Sequence Number Format

The ID is a zero-padded incrementing hex counter in UUID format:
- `00000000-0000-0000-0000-000000000001`
- `00000000-0000-0000-0000-000000000002`
- etc.

The **outer header bytes 2-3** contain the same hex sequence number (e.g., `00 05` for request ID `...000000000005`).

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

The device MAC is formatted as **12 lowercase hex characters without separators**.

Example: MAC `DE:AD:BE:EF:CA:FE` becomes `deadbeefcafe`

---

## API Endpoints

All endpoints (except `/api/version`) use the base path: `/api/1.0/{mac}/`

### GET `/api/version`

Returns firmware and API version info.

**Response:**
```json
{
  "fwv": "1.1.1",
  "apiVersion": "1.0"
}
```

---

### GET `/api/1.0/{mac}`

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

---

### GET `/api/1.0/{mac}/stats`

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

---

### GET `/api/1.0/{mac}/settings`

Returns device settings.

**Response:**
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

---

### GET `/api/1.0/{mac}/bt`

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

### GET `/api/1.0/{mac}/fw`

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

---

### SIF (SFP Interface) Operations

The SIF (SFP Interface) protocol is used to read and write SFP module EEPROM data.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/1.0/{mac}/sif/start` | Start SIF read operation |
| GET | `/api/1.0/{mac}/sif/info/` | SIF operation status |
| GET | `/api/1.0/{mac}/sif/data/` | Read SIF data chunk |
| POST | `/api/1.0/{mac}/sif/abort` | Abort SIF operation |

**Note:** The `/sif/info/` and `/sif/data/` paths require a trailing slash.

#### POST `/api/1.0/{mac}/sif/start`

Initiates an SIF read operation. Must be called before reading data.

**Request body:** None (empty)

**Response:**
```json
{
  "status": "ready",
  "offset": 0,
  "chunk": 1024,
  "size": 512
}
```

| Field | Description |
|-------|-------------|
| `status` | "ready" when initialized |
| `offset` | Starting offset (always 0) |
| `chunk` | Maximum chunk size for data requests |
| `size` | Total EEPROM size in bytes (512 for SFP A0h+A2h pages) |

#### GET `/api/1.0/{mac}/sif/data/`

Reads a chunk of EEPROM data. Call in a loop until all data is retrieved.

**Request body:**
```json
{
  "status": "continue",
  "offset": 0,
  "chunk": 512
}
```

| Field | Description |
|-------|-------------|
| `status` | Must be "continue" |
| `offset` | Byte offset to read from |
| `chunk` | Number of bytes to read (max = chunk size from start response) |

**Response body:** Raw binary EEPROM data (not JSON).

**Important:** Large responses are **fragmented across multiple BLE notifications**. The outer header's total length field indicates the complete message size. Accumulate notification payloads until the total length is reached.

#### GET `/api/1.0/{mac}/sif/info/`

Returns the current SIF operation status.

**Request body:** None (empty)

**Response:**
```json
{
  "status": "finished",
  "offset": 512
}
```

| Field | Description |
|-------|-------------|
| `status` | Status string (see below) |
| `offset` | Current read offset |

**Status values:**
- `ready` - Initialized, ready to read data
- `continue` - Read in progress, more data available
- `inprogress` - Actively reading
- `complete` - Read finished successfully
- `finished` - Read finished successfully (alternate)

#### SIF Read Flow

1. **POST `/sif/start`** - Initialize, get total size and chunk size
2. **Loop: GET `/sif/data/`** with offset/chunk body - Fetch data in chunks
3. **GET `/sif/info/`** - Verify completion (optional)

Example read sequence:
```
POST /sif/start           → {"status":"ready","offset":0,"chunk":1024,"size":512}
GET  /sif/data/ (0,512)   → [512 bytes of raw EEPROM data]
GET  /sif/info/           → {"status":"complete","offset":512}
```

#### BLE Response Fragmentation

SIF data responses can be large (512+ bytes) and exceed the BLE MTU. The device fragments these across multiple notifications:

1. First notification contains the outer header with total length
2. Subsequent notifications contain continuation data
3. Accumulate all fragments until `total_received >= total_length`

Example: A 600-byte response might arrive as:
- Notification 1: 244 bytes (header + start of data)
- Notification 2: 244 bytes (continuation)
- Notification 3: 112 bytes (final fragment)

#### SIF Archive Contents

The SIF data is returned as a **tar archive** containing:

| File | Size | Description |
|------|------|-------------|
| `syslog` | ~5-10KB | Device logs (grows over time, clears on reboot) |
| `sfp_primary.bin` | 512 | SFP module read via device screen (physical button) |
| `sfp_secondary.bin` | 512 | SFP module read via API (`/sif/start`) |
| `qsfp_primary.bin` | 640 | QSFP module read via device screen (0xff if empty) |
| `qsfp_secondary.bin` | 640 | QSFP module read via API |
| `{PartNumber}.bin` | 512/640 | Module database entries (keyed by S/N, named by PN suffix) |

**Notes:**
- `primary` = read initiated via device screen, `secondary` = read initiated via API
- The named `{PartNumber}.bin` files persist across reboots (stored in flash)
- Database keys by **serial number**, with 2 slots per unique module (screen + API)
- Filename is the part number suffix, so multiple modules can share the same filename
- Files filled with `0xff` indicate no module present in that slot
- EEPROM format follows SFF-8472 (SFP) or SFF-8636 (QSFP) specifications

---

### POST `/api/1.0/{mac}/reboot`

Reboots the device. The BLE connection will drop during reboot.

**Request body:** None (empty)

**Response:** Status 200 on success (connection may drop before response is received)

---

### Other Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/1.0/{mac}/name` | Set device name |
| POST | `/api/1.0/{mac}/fw/start` | Start firmware update |
| POST | `/api/1.0/{mac}/fw/data` | Send firmware chunk |
| POST | `/api/1.0/{mac}/fw/abort` | Abort firmware update |

---

### XSFP (Extended SFP) Operations

The XSFP protocol provides direct read/write access to SFP module EEPROM "snapshots". Unlike the SIF protocol which returns a tar archive, XSFP works with raw binary data and supports **writing** to module EEPROM.

**Note:** These endpoints are registered as custom handlers and may not be available on all firmware versions.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/1.0/{mac}/xsfp/sync/start` | Get current snapshot info |
| POST | `/api/1.0/{mac}/xsfp/sync/start` | Initialize write transfer |
| GET | `/api/1.0/{mac}/xsfp/sync/data` | Read snapshot data |
| POST | `/api/1.0/{mac}/xsfp/sync/data` | Write snapshot data chunk |
| POST | `/api/1.0/{mac}/xsfp/sync/cancel` | Cancel transfer |
| GET | `/api/1.0/{mac}/xsfp/module/start` | Start module read |
| GET | `/api/1.0/{mac}/xsfp/module/data` | Read module data |
| GET | `/api/1.0/{mac}/xsfp/module/details` | Get module details |
| POST | `/api/1.0/{mac}/xsfp/recover` | Recovery operation |

#### Snapshot Sizes

| Size | Module Type |
|------|-------------|
| 512 bytes (0x200) | SFP module (A0h + A2h pages) |
| 640 bytes (0x280) | QSFP module |

#### XSFP Write Flow

To write EEPROM data to an SFP module:

1. **POST `/xsfp/sync/start`** - Initialize transfer with expected size
   ```json
   {"size": 512}
   ```
   Response confirms the transfer is ready.

2. **POST `/xsfp/sync/data`** - Send binary data chunks
   - Request body: Raw binary EEPROM data
   - Server tracks `received` vs `expected` bytes
   - Continue sending until all data is transferred

3. **On completion** - Device validates and applies the snapshot
   - Fires `xsfp_load_completed` event internally
   - Returns JSON status response

#### XSFP Read Flow

To read current module EEPROM:

1. **GET `/xsfp/sync/start`** or **GET `/xsfp/module/start`** - Initialize read
2. **GET `/xsfp/sync/data`** or **GET `/xsfp/module/data`** - Fetch data chunks

#### Error Handling

| Status Code | Meaning |
|-------------|---------|
| 200 | Success |
| 400 | Invalid request/argument |
| 500 | Internal error / allocation failure |
| 0x130 (304) | Invalid snapshot data |
| 0x19d (413) | Data size mismatch |
| 0x1a1 (417) | Unexpected snapshot size |

---

## Example Request Packet

Raw request to `/api/1.0/deadbeefcafe/stats`:

```
Outer header:    00 9a 00 05
                 ^^^^^       - total length (154 bytes)
                       ^^^^^ - sequence number (5)

Header section:  03 01 01 01 00 00 00 00 7d
                 ^^ - marker (header)
                    ^^ - format (JSON)
                       ^^ - compression (zlib)?
                          ^^ - flags
                             ^^^^^^^^^^^ - reserved
                                         ^^ - compressed length (125 bytes)
                 [125 bytes of zlib compressed JSON]

Body section:    02 01 01 00 00 00 00 08
                 ^^ - marker (body)
                    ^^ - format (JSON)
                       ^^ - compression (zlib)?
                          ^^ - reserved
                             ^^^^^^^^^^^ - length (8 bytes)
                 78 9c 03 00 00 00 00 01   (compressed empty body)
```

---

## Example Response Packet

Raw response from `/api/version`:

```
Outer header:    00 b2 00 01
                 ^^^^^       - total length (178 bytes)
                       ^^^^^ - sequence number (1)

Header section:  03 01 01 00 00 00 00 00 7b
                 ^^ - marker (header)
                    ^^ - format (JSON)
                       ^^ - compression flag? (but NOT actually compressed!)
                          ^^ - flags (0x00 for response)
                             ^^^^^^^^^^^ - reserved
                                         ^^ - data length (123 bytes)
                 [123 bytes of RAW JSON - not compressed despite flag]

Body section:    02 01 00 00 00 00 00 22
                 ^^ - marker (body)
                    ^^ - format (JSON)
                       ^^ - compression (none)?
                          ^^ - reserved
                             ^^^^^^^^^^^ - length (34 bytes)
                 {"fwv":"1.1.1","apiVersion":"1.0"}
```
