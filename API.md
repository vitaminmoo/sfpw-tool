# UACC SFP Wizard BLE API Protocol

This document describes the BLE API protocol for the UACC SFP Wizard device.

Note that it is primarily maintained via LLM and certainly contains errors.

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

## Protocol Overview

The protocol uses **JSON messages wrapped in a binary envelope over BLE GATT**, with **zlib compression** for requests.

### BLE Characteristics

| Service UUID                         | Characteristic UUID                  | Handle | Purpose                    |
| ------------------------------------ | ------------------------------------ | ------ | -------------------------- |
| 8E60F02E-F699-4865-B83F-F40501752184 | 9280F26C-A56F-43EA-B769-D5D732E1AC67 | 0x10   | Write requests             |
| 8E60F02E-F699-4865-B83F-F40501752184 | DC272A22-43F2-416B-8FA5-63A071542FAC | 0x11   | Device info (read)         |
| 8E60F02E-F699-4865-B83F-F40501752184 | D587C47F-AC6E-4388-A31C-E6CD380BA043 | 0x15   | **API responses (notify)** |

**Important:** Subscribe to `D587C47F` for API responses, NOT `DC272A22`.

### Binary Envelope Format

Messages use a device transport header followed by **modified binme** binary envelope sections.

**Note:** The SFP Wizard device uses a modified version of the binme protocol. Key differences from standard binme:
- Header section uses type `0x03` instead of standard `0x01`
- Header section is 9 bytes (vs standard 8), with single-byte length at byte 8
- Body section matches standard binme format

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

**Binme Constants (standard library values):**

| Constant      | Value | Description                                |
| ------------- | ----- | ------------------------------------------ |
| TYPE_HEADER   | 0x01  | Header section type (standard binme)       |
| TYPE_BODY     | 0x02  | Body section type                          |
| FORMAT_JSON   | 0x01  | JSON data format                           |
| FORMAT_STRING | 0x02  | UTF-8 string format                        |
| FORMAT_BINARY | 0x03  | Raw binary format                          |

**Device-specific values:**

| Field              | Value | Description                                |
| ------------------ | ----- | ------------------------------------------ |
| Device header type | 0x03  | Device uses 0x03 instead of standard 0x01  |

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

| Field        | Type   | Description                                                      |
| ------------ | ------ | ---------------------------------------------------------------- |
| `type`       | string | `httpRequest` or `httpResponse`                                  |
| `id`         | string | Request ID for correlation (incrementing counter in UUID format) |
| `timestamp`  | number | Unix timestamp in milliseconds                                   |
| `method`     | string | HTTP method: `GET` or `POST` (request only)                      |
| `path`       | string | API endpoint path (request only)                                 |
| `statusCode` | number | HTTP status code (response only)                                 |
| `headers`    | object | Always empty `{}`                                                |

### ID and Sequence Number Format

The ID is a zero-padded incrementing hex counter in UUID format:

- `00000000-0000-0000-0000-000000000001`
- `00000000-0000-0000-0000-000000000002`
- etc.

The **outer header bytes 2-3** contain the same hex sequence number (e.g., `00 05` for request ID `...000000000005`).

### Status Codes

| Code | Meaning               |
| ---- | --------------------- |
| 200  | Success               |
| 304  | Not Modified          |
| 400  | Bad Request           |
| 404  | Not Found             |
| 500  | Internal Server Error |

---

## MAC Address Format

The device MAC is formatted as **12 lowercase hex characters without separators**.

Example: MAC `DE:AD:BE:EF:CA:FE` becomes `deadbeefcafe`

---

## API Endpoints

All endpoints (except `/api/version`) use the base path: `/api/1.0/{mac}/`

### GET `/api/version`

Returns firmware and API version info.

**Note:** This endpoint returns 404 on firmware versions 1.0.10 and 1.1.0. Availability on other versions is unknown.

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

| Method | Path                       | Description              |
| ------ | -------------------------- | ------------------------ |
| POST   | `/api/1.0/{mac}/sif/start` | Start SIF read operation |
| GET    | `/api/1.0/{mac}/sif/info/` | SIF operation status     |
| GET    | `/api/1.0/{mac}/sif/data/` | Read SIF data chunk      |
| POST   | `/api/1.0/{mac}/sif/abort` | Abort SIF operation      |

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

| Field    | Description                                            |
| -------- | ------------------------------------------------------ |
| `status` | "ready" when initialized                               |
| `offset` | Starting offset (always 0)                             |
| `chunk`  | Maximum chunk size for data requests                   |
| `size`   | Total EEPROM size in bytes (512 for SFP A0h+A2h pages) |

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

| Field    | Description                                                    |
| -------- | -------------------------------------------------------------- |
| `status` | Must be "continue"                                             |
| `offset` | Byte offset to read from                                       |
| `chunk`  | Number of bytes to read (max = chunk size from start response) |

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

| Field    | Description               |
| -------- | ------------------------- |
| `status` | Status string (see below) |
| `offset` | Current read offset       |

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

| File                 | Size    | Description                                                |
| -------------------- | ------- | ---------------------------------------------------------- |
| `syslog`             | ~5-10KB | Device logs (grows over time, clears on reboot)            |
| `sfp_primary.bin`    | 512     | SFP module read via device screen (physical button)        |
| `sfp_secondary.bin`  | 512     | SFP module read via API (`/sif/start`)                     |
| `qsfp_primary.bin`   | 640     | QSFP module read via device screen (0xff if empty)         |
| `qsfp_secondary.bin` | 640     | QSFP module read via API                                   |
| `{PartNumber}.bin`   | 512/640 | Module database entries (keyed by S/N, named by PN suffix) |

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

| Method | Path                      | Description           |
| ------ | ------------------------- | --------------------- |
| POST   | `/api/1.0/{mac}/name`     | Set device name       |
| POST   | `/api/1.0/{mac}/fw/start` | Start firmware update |
| POST   | `/api/1.0/{mac}/fw/data`  | Send firmware chunk   |
| POST   | `/api/1.0/{mac}/fw/abort` | Abort firmware update |

---

### XSFP (Extended SFP) Operations

The XSFP protocol provides direct read/write access to SFP module EEPROM "snapshots". Unlike the SIF protocol which returns a tar archive, XSFP works with raw binary data and supports **writing** to module EEPROM.

**Version Notes:**

- `/xsfp/module/details` is **new in 1.1.0** (returns 404 on 1.0.10)
- Other XSFP endpoints work on both 1.0.10 and 1.1.0
- All XSFP read operations return 417 (Expectation Failed) if no module is inserted

| Method | Path                                 | Description               |
| ------ | ------------------------------------ | ------------------------- |
| GET    | `/api/1.0/{mac}/xsfp/sync/start`     | Get current snapshot info |
| POST   | `/api/1.0/{mac}/xsfp/sync/start`     | Initialize write transfer |
| GET    | `/api/1.0/{mac}/xsfp/sync/data`      | Read snapshot data        |
| POST   | `/api/1.0/{mac}/xsfp/sync/data`      | Write snapshot data chunk |
| POST   | `/api/1.0/{mac}/xsfp/sync/cancel`    | Cancel transfer           |
| GET    | `/api/1.0/{mac}/xsfp/module/start`   | Start module read         |
| GET    | `/api/1.0/{mac}/xsfp/module/data`    | Read module data          |
| GET    | `/api/1.0/{mac}/xsfp/module/details` | Get module details        |
| POST   | `/api/1.0/{mac}/xsfp/recover`        | Recovery operation        |

#### GET `/api/1.0/{mac}/xsfp/module/details`

Returns details about the currently inserted SFP module without reading the full EEPROM. Fast way to check module presence and identity.

**Requires:** Firmware 1.1.0+

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

| Field        | Description                                                 |
| ------------ | ----------------------------------------------------------- |
| `partNumber` | Module part number                                          |
| `rev`        | Revision                                                    |
| `vendor`     | Vendor name/ID                                              |
| `sn`         | Serial number                                               |
| `type`       | Module type: "sfp" or "qsfp" (1.1.1+)                       |
| `compliance` | Transceiver compliance (e.g., "10G BASE-SR", "10G BASE-LR") |

**Note:** Returns 404 if no module is inserted.

#### GET `/api/1.0/{mac}/xsfp/sync/start`

Returns information about the current snapshot buffer contents.

**Requires:** Firmware 1.1.0+

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

| Field        | Description                           |
| ------------ | ------------------------------------- |
| `partNumber` | Part number of module in snapshot     |
| `vendor`     | Vendor of module in snapshot          |
| `sn`         | Serial number of module in snapshot   |
| `type`       | Module type: "sfp" or "qsfp" (1.1.1+) |
| `chunk`      | Maximum chunk size for data transfer  |
| `size`       | Total size of snapshot data in bytes  |

**Note:** Returns 417 (Expectation Failed) if no module is inserted.

#### Snapshot Sizes

| Size              | Module Type                  |
| ----------------- | ---------------------------- |
| 512 bytes (0x200) | SFP module (A0h + A2h pages) |
| 640 bytes (0x280) | QSFP module                  |

#### XSFP Write Flow

To write EEPROM data to an SFP module:

1. **POST `/xsfp/sync/start`** - Initialize transfer with expected size

   ```json
   { "size": 512 }
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

| Status Code | Meaning                             |
| ----------- | ----------------------------------- |
| 200         | Success                             |
| 400         | Invalid request/argument            |
| 500         | Internal error / allocation failure |
| 0x130 (304) | Invalid snapshot data               |
| 0x19d (413) | Data size mismatch                  |
| 0x1a1 (417) | Unexpected snapshot size            |

#### POST `/api/1.0/{mac}/xsfp/recover`

Restores a module's EEPROM from a previously saved "golden snapshot" in the device database.

**Request body:**

```json
{
  "sn": "SERIALNUMBER",
  "wavelength": 1310
}
```

| Field        | Type   | Required | Description                                    |
| ------------ | ------ | -------- | ---------------------------------------------- |
| `sn`         | string | Yes      | Serial number of the module to recover         |
| `wavelength` | number | No       | Override wavelength value in restored snapshot |

**Response:**

- **200**: Success - snapshot restored, fires `xsfp_load_completed` event internally
- **404**: Golden snapshot not found for the given serial number

**Error message (404):**

```
Golden snapshot appears to be invalid or not found for sn:'SERIALNUMBER'
```

**Notes:**

- Looks up the golden snapshot by serial number in the device's persistent database
- Validates snapshot size (512 for SFP, 640 for QSFP)
- If `wavelength` is provided, overrides the wavelength field in the restored data

---

## Module Database

The device maintains a persistent database of module snapshots on its internal flash storage.

### Storage Implementation

- **Filesystem:** LittleFS (Little File System) on ESP32-S3 flash
- **Location:** Dedicated flash partition
- **Format:** Individual binary files per module

### Database Structure

| Aspect               | Details                                     |
| -------------------- | ------------------------------------------- |
| **Key**              | Module serial number                        |
| **Filename**         | Part number suffix (e.g., `SFP-10G-SR.bin`) |
| **File size**        | 512 bytes (SFP) or 640 bytes (QSFP)         |
| **Slots per module** | 2 (one for screen read, one for API read)   |

### Storage Limits

- **No hardcoded limit** on number of modules
- Limited only by flash partition size
- Each module entry uses 512-640 bytes plus filesystem overhead
- Typical flash partitions can store hundreds of module snapshots

### Database Operations

| Operation    | Method                                             |
| ------------ | -------------------------------------------------- |
| **Save**     | Automatic when module is read (via screen or API)  |
| **Retrieve** | Via `/xsfp/recover` endpoint with serial number    |
| **List**     | Via SIF archive - database files included in tar   |
| **Clear**    | Device reboot clears runtime cache; flash persists |

### File Naming

Multiple modules can share the same filename if they have identical part numbers. The database internally keys by serial number, so modules with the same part number but different serial numbers are stored separately despite having the same filename in the SIF archive export.

---

## SFP Password Database

The firmware contains an embedded password database used to unlock locked SFP/QSFP modules. This database maps part numbers to their unlock passwords.

### Database Format

The password database is stored in the DROM segment of the ESP32 firmware image. The entry structure changed between firmware versions:

| Firmware     | Entry Size | Fields                                                                |
| ------------ | ---------- | --------------------------------------------------------------------- |
| 1.0.x, 1.1.0 | 20 bytes   | read_only, part_number\*, locked, password[4], flags[3], cable_length |
| 1.1.1+       | 16 bytes   | read_only, part_number\*, locked, password[4], flags[3]               |

**Entry Structure (1.0.x, 1.1.0 - 20 bytes):**

```
Offset  Size  Field
0x00    4     read_only (uint32) - Skip if non-zero
0x04    4     part_number (char*) - Pointer to null-terminated string
0x08    1     locked (bool) - Module requires unlock
0x09    4     password[4] - 4-byte unlock password
0x0D    3     flags[3] - Module flags
0x10    4     cable_length (int32) - Cable length or reach in meters
```

The `cable_length` field served a dual purpose in older firmware - the password database was also used as a module metadata lookup table:

- **DAC/AOC cables**: Physical cable length (e.g., 5M cable → 5)
- **Optical modules**: Maximum transmission reach (e.g., LR → 10000 for 10km)

**Entry Structure (1.1.1+ - 16 bytes):**

```
Offset  Size  Field
0x00    4     read_only (uint32) - Skip if non-zero
0x04    4     part_number (char*) - Pointer to null-terminated string
0x08    1     locked (bool) - Module requires unlock
0x09    4     password[4] - 4-byte unlock password
0x0D    3     flags[3] - Module flags
```

In 1.1.1+, the `cable_length` field was removed, separating the password database from module metadata. Cable length is likely now parsed from the part number string or stored elsewhere.

The database is terminated by an entry with a NULL `part_number` pointer.

### Flags Field

The `flags[0]` byte is a bitmask indicating which EEPROM pages can be written after unlock:

| Bit | Value | Page          | Description                              |
| --- | ----- | ------------- | ---------------------------------------- |
| 0   | 0x01  | A0h / Lower   | Basic identity page (all modules)        |
| 1   | 0x02  | A2h / Upper 1 | SFP diagnostic page or QSFP upper page 1 |
| 2   | 0x04  | Upper 2       | QSFP upper page 2                        |
| 3   | 0x08  | Upper 3       | QSFP upper page 3 (thresholds)           |

**Common flag values:**

| Value | Binary | Meaning                       |
| ----- | ------ | ----------------------------- |
| 0x01  | `0001` | Lower page only (minimal)     |
| 0x03  | `0011` | SFP modules (A0h + A2h)       |
| 0x0D  | `1101` | Some QSFP (lower + upper 2,3) |
| 0x0F  | `1111` | Full QSFP (all 4 pages)       |

The firmware checks these flags before writing to a property - if the corresponding bit isn't set, the write is skipped. Properties like `pu0vn` (vendor name), `pu3txhat` (TX high alarm threshold) reference specific pages.

`flags[1]` and `flags[2]` are less commonly used and their meaning is not fully understood.

### Password Lookup Algorithm (1.0.10)

1. Look up module by part number string using `strcmp` (exact match, first match wins)
2. If not found, try alternate part number (vendor PN)
3. If still not found, use default entry (last entry with NULL part_number)
4. If entry is `read_only`, skip unlock attempt
5. If entry is not `locked`, skip unlock (module doesn't need password)
6. Send 4-byte password to module's password register (A2h offset 0x7B)
7. If unlock fails, fallback: collect ALL unique passwords from entire database and try each

**Key functions:**

| Function                               | Purpose                                                 |
| -------------------------------------- | ------------------------------------------------------- |
| `sfp_password_db_lookup_by_partnumber` | strcmp lookup, returns first match                      |
| `sfp_collect_unique_passwords`         | Collects all unique passwords from entire DB (fallback) |

### Password Lookup Algorithm (1.1.3)

1. Collect ALL database entries matching the part number using `strcmp`
2. If no matches, try alternate part number (vendor PN)
3. If still no matches, use default entry
4. Deduplicate collected entries by password value (4-byte comparison)
5. Try each unique password in sequence until unlock succeeds

**Key difference from 1.0.10:** The fallback mechanism changed. In 1.0.10, if the first-match password fails, the device tries ALL unique passwords from the entire database. In 1.1.3, it only tries passwords from entries that match the module's part number.

### Database Content Differences in 1.1.1+

Starting in 1.1.1, some modules have **multiple database entries** with different passwords. This allows the device to try alternate passwords if the first one fails:

| Part Number | 1.0.10 Passwords | 1.1.1+ Passwords             |
| ----------- | ---------------- | ---------------------------- |
| OM-MM-10G-D | `00 00 10 11`    | `00 00 10 11`, `63 73 77 77` |
| OM-SM-10G-D | `00 00 10 11`    | `63 73 77 77`, `00 00 10 11` |
| OM-SFP28-SR | `00 00 10 11`    | `00 00 10 11`, `63 73 77 77` |
| OM-SFP28-LR | `53 46 50 58`    | `53 46 50 58`, `63 73 77 77` |

### Database Summary by Firmware Version

| Firmware | Entries | Entry Size | Unique Passwords |
| -------- | ------- | ---------- | ---------------- |
| 1.0.10   | 54      | 20 bytes   | 5                |
| 1.1.0    | 54      | 20 bytes   | 5                |
| 1.1.1    | 58      | 16 bytes   | 6                |
| 1.1.3    | 59      | 16 bytes   | 6                |

### Known Passwords

| Password (Hex) | Password (ASCII) | Used By                                    | Firmware |
| -------------- | ---------------- | ------------------------------------------ | -------- |
| `00 00 10 11`  | -                | Most AOC/Uplink/OM modules (default)       | All      |
| `78 56 34 12`  | -                | DAC-SFP28-3M, OM-SFP10-12xx to 15xx (DWDM) | All      |
| `53 46 50 58`  | "SFPX"           | OM-SFP28-LR                                | All      |
| `80 81 82 83`  | -                | OM-QSFP28-LR4, OM-QSFP28-PSM4              | All      |
| `51 53 46 50`  | "QSFP"           | OM-QSFP28-SR4                              | All      |
| `63 73 77 77`  | "csww"           | Alternate for OM-MM/SM/SFP28 modules       | 1.1.1+   |

### Extracting the Password Database

Use `sfpw fw passdb` to extract the password database from a firmware image:

```bash
# Extract and display password database
sfpw fw passdb firmware.bin

# Output as JSON
sfpw fw passdb -j firmware.bin

# Evaluate a specific part number to see what passwords would be tried
sfpw fw passdb -s "OM-SFP28-LR" firmware.bin
```

---

## Example Request Packet

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
                               ^^^^^^^^^^^ - length (8 bytes, big-endian uint32)
                   78 9c 03 00 00 00 00 01   (zlib compressed empty body)
```

---

## Example Response Packet

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
                   [123 bytes of RAW JSON - not compressed despite isCompressed flag]

Body section:      02 01 00 00 00 00 00 22
                   ^^ - type (0x02 = TYPE_BODY)
                      ^^ - format (0x01 = FORMAT_JSON)
                         ^^ - isCompressed (0x00 = none)
                            ^^ - reserved
                               ^^^^^^^^^^^ - length (34 bytes, big-endian uint32)
                   {"fwv":"1.1.1","apiVersion":"1.0"}
```
