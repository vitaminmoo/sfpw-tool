# SFP Wizard Flasher - TODO

## Project Overview
Go CLI tool for interacting with the SFP Wizard (Ubiquiti) hardware device over Bluetooth Low Energy (BLE).

**Target Device:** SFP Wizard (UACC-SFP-Wizard)
**Device Firmware:** v1.1.1
**API Version:** 1.0 (as reported by device)

---

## ✅ Confirmed Working

### Core BLE Functionality
- [x] BLE adapter initialization and device scanning
- [x] Device discovery (case-insensitive name matching)
- [x] Connection handling with service discovery
- [x] Reading device info from notify characteristic (returns JSON)

### What We Know Works
- **Reading dc272a22... characteristic** returns device JSON:
  ```json
  {"id":"1C6A1B7FBE88","fwv":"1.1.1","apiVersion":"1.0","voltage":"4158","level":"100"}
  ```

---

## ❌ Critical Issue Discovered

### Device Enters Bad State After Our Connection

**Problem:** After connecting with our tool (Go or Python), the SFP Wizard stops responding to the official Ubiquiti mobile app until the device is rebooted.

**Implications:**
- Our connection method is corrupting device state
- All previous testing was likely invalid (device was in bad state)
- Need to understand what the official app does differently

### Possible Causes
1. **Subscription method** - Maybe we're subscribing incorrectly
2. **Write method** - Maybe we're writing to wrong characteristic or wrong format
3. **Connection parameters** - MTU, connection interval, etc.
4. **Missing initialization** - Some handshake or auth we're not doing
5. **Characteristic handles** - Duplicate UUIDs across services causing confusion

---

## 🔍 BLE Service/Characteristic Map

```
Service #3: 8e60f02e-f699-4865-b83f-f40501752184 (SFP Service)
  Handle 15: 9280f26c-a56f-43ea-b769-d5d732e1ac67 [write-without-response, write]
  Handle 17: dc272a22-43f2-416b-8fa5-63a071542fac [notify, read, write-without-response, write]
  Handle 20: d587c47f-ac6e-4388-a31c-e6cd380ba043 [notify, read]

Service #4: 0b9676ee-8352-440a-bf80-61541d578fcf (Unknown - v1.1.1?)
  Handle 24: 9280f26c-a56f-43ea-b769-d5d732e1ac67 [write] (SAME UUID as Service #3!)
  Handle 26: d587c47f-ac6e-4388-a31c-e6cd380ba043 [notify, read] (SAME UUID as Service #3!)
```

**Note:** Duplicate UUIDs across services is unusual and may be significant.

---

## 📋 Next Steps

### Reverse Engineering Required
1. [ ] **Reverse engineer official Ubiquiti app**
   - Capture BLE traffic with Wireshark/nRF Sniffer
   - Decompile APK and analyze BLE code
   - Document exact connection sequence
   - Document command format and responses

2. [ ] **Reverse engineer device firmware**
   - Analyze firmware binary
   - Understand BLE message handling
   - Find what causes "bad state"

3. [ ] **Compare connection sequences**
   - What does official app do that we don't?
   - MTU negotiation?
   - Connection parameters?
   - Specific characteristic order?

### After RE Complete
4. [ ] Implement correct connection sequence
5. [ ] Implement working command protocol
6. [ ] Test EEPROM read
7. [ ] Test EEPROM write

---

## 📚 References

- **Third-party v1.0.10 code** - Uses simple text commands but for older firmware
- **Device logs** - Show command format but from internal perspective
- **BLE Spec v1.0.10** - Outdated, device is v1.1.1

---

## 💡 Hints from Device Logs

Commands seen in device logs (may include internal formatting):
```
/api/1.0/version
[GET] /stats[0]
[POST] /sif/start[0]
[GET] /sif/data[45]{"status":"continue","offset":0,"chunk":4096}
[GET] /sif/info[0]
```

**Note:** The `[0]` and `[N]` suffixes may be internal logging, not part of actual commands.

---

Last Updated: 2026-01-14
