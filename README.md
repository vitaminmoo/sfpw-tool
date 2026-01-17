# sfpw-tool

A command-line tool for interacting with the Ubiquiti SFP Wizard (UACC-SFP-Wizard) over Bluetooth Low Energy.

The SFP Wizard is a portable device for reading, writing, and cloning SFP/SFP+/SFP28 module EEPROMs. sfpw-tool provides full control over the device from Linux, macOS, and other platforms supported by TinyGo's bluetooth library.

> [!CAUTION]
> **This is a reverse-engineered third-party client.** I take no responsibility for dead SFP Wizards, bricked SFP modules, or exploded computers. If it breaks, you get to keep all the pieces.

## Features

- **Device Management** - Query device info, battery status, firmware version, and settings
- **Module Operations** - Read EEPROM from inserted SFP modules, write EEPROM data via the snapshot buffer
- **Firmware Management** - Download firmware from Ubiquiti's servers, update device firmware over BLE
- **Password Database** - Extract, inspect, and search the built-in SFP password database from firmware images
- **Profile Store** - Local content-addressed storage for module EEPROM profiles
- **EEPROM Parsing** - Offline parsing of SFF-8472 (SFP) and SFF-8636 (QSFP) EEPROM dumps

## Installation

```bash
go install github.com/vitaminmoo/sfpw-tool@latest
```

## Usage

### Device Information

```bash
# Get device info
$ sfpw-tool device info
Scanning for SFP Wizard...
Connecting to DE:AD:BE:EF:CA:FE...
Connected!
{
  "id": "DEADBEEFCAFE",
  "type": "Usfpw-tool",
  "fwv": "1.1.3",
  "bomId": "10652-8",
  "proId": "9487-1",
  "state": "app",
  "name": "Sfp Wizard"
}

# Get battery and uptime stats
$ sfpw-tool device stats
Scanning for SFP Wizard...
Connecting to DE:AD:BE:EF:CA:FE...
Connected!
Battery:      78% (3.966V)
Low Battery:  false
Uptime:       6d 20h
Signal:       0 dBm
```

### Module Operations

The SFP Wizard has a "snapshot buffer" for each module type (SFP, QSFP, etc.):

- **`module read`** - Reads EEPROM directly from the inserted module
- **`snapshot read`** - Reads the snapshot buffer (populated when you press Copy on the device screen)
- **`snapshot write`** - Writes EEPROM data to the snapshot buffer (like pressing Copy, but with your data instead of the inserted module's)

To flash EEPROM to a module: run `snapshot write`, then **press Write on the device screen**.

```bash
# Read EEPROM directly from inserted module
$ sfpw-tool module read output.bin

# Read the snapshot buffer (last Copy from device screen)
$ sfpw-tool snapshot read output.bin

# Get snapshot buffer info
$ sfpw-tool snapshot info

# Write EEPROM to snapshot buffer (from file)
# Then press Write on device screen to flash to module
$ sfpw-tool snapshot write input.bin

# Write EEPROM to snapshot buffer (from stored profile hash)
$ sfpw-tool snapshot write abc1234
```

### Firmware Management

> [!WARNING]
> While the OTA update process is implemented very safely on the device itself, there is not zero risk of damage. Proceed with caution.

Downgrades work safely - tested between v1.1.3 and v1.0.10 on hardware version 8.

```bash
# Download all available firmware versions
$ sfpw-tool fw download

# List downloaded firmware
$ sfpw-tool fw list
Downloaded firmware files (4):

  v1.0.10       1.9 MB      2025-10-28 05:43
  v1.1.0        1.9 MB      2025-11-14 00:09
  v1.1.1        1.9 MB      2025-11-21 04:44
  v1.1.3        1.8 MB      2026-01-08 07:45

# Update device firmware (accepts file path or version)
$ sfpw-tool fw update v1.1.3

# Update from a local firmware file
$ sfpw-tool fw update 1.0.5.bin

# Check firmware status
$ sfpw-tool fw status
```

### Password Database

The SFP Wizard firmware contains a database of known SFP module passwords for unlocking protected EEPROMs.

```bash
# Extract password database from firmware
$ sfpw-tool fw passdb v1.1.3
Using downloaded firmware: v1.1.3
Password Database: 1.1.x (16-byte entries)
Entry size: 16 bytes
Total entries: 59

Unique Passwords:
--------------------------------------------------
  00 00 10 11
  78 56 34 12
  63 73 77 77 ("csww")
  53 46 50 58 ("SFPX")
  ...

# Search for passwords for a specific part number
$ sfpw-tool fw passdb v1.1.3 -s "UACC-UF-OM-XGS"
```

### Profile Store

EEPROM profiles are stored locally in `~/.local/share/sfpw-tool/store/`.

```bash
# List stored profiles
$ sfpw-tool store list

# Import an EEPROM file
$ sfpw-tool store import module.bin

# Export a profile by hash
$ sfpw-tool store export abc123 output.bin
```

### Offline EEPROM Parsing

```bash
# Parse an EEPROM dump without connecting to the device
$ sfpw-tool debug parse-eeprom module.bin
```

## Data Storage

- **Firmware**: `~/.local/share/sfpw-tool/firmware/`
- **Module Profiles**: `~/.local/share/sfpw-tool/store/`

## Requirements

- Go 1.24.4 or later
- Bluetooth adapter with BLE support
- Appropriate permissions for Bluetooth access

## TODO

- [ ] Implement recovery endpoint (writes the EEPROM to the device by looking up its serial number and writing the profile from an archive kept on the device that includes everything you've ever read)
- [ ] Finish TUI

## Notes

> [!NOTE]
> I do not own any QSFP modules and have not been able to test QSFP functionality.

> [!NOTE]
> I'm looking for firmware versions older than v1.0.10. If anyone has torn open a device and extracted an older firmware physically, please get in touch!
