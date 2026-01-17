# SFP Wizard GPIO Configuration

This document describes the GPIO pin assignments and usage in the Ubiquiti SFP Wizard (UACC-SFP-Wizard) firmware, based on reverse engineering of firmware version 1.0.10.

> [!CAUTION]
> Discovered, generated and maintained by an LLM, trust as far as you can throw.

## Hardware Overview

The SFP Wizard uses an **ESP32-S3** microcontroller with:

- Direct ESP32-S3 GPIO pins for primary I/O
- **CA9554** I2C I/O expander for additional GPIO (some SFP-related signals)

## ESP32-S3 GPIO Register Addresses

| Register           | Address    | Description                               |
| ------------------ | ---------- | ----------------------------------------- |
| GPIO_OUT_REG       | 0x60004004 | GPIO 0-31 output register                 |
| GPIO_OUT_W1TS_REG  | 0x60004008 | GPIO 0-31 output set (write 1 to set)     |
| GPIO_OUT_W1TC_REG  | 0x6000400c | GPIO 0-31 output clear (write 1 to clear) |
| GPIO_OUT1_REG      | 0x60004010 | GPIO 32-53 output register                |
| GPIO_OUT1_W1TS_REG | 0x60004014 | GPIO 32-53 output set                     |
| GPIO_OUT1_W1TC_REG | 0x60004018 | GPIO 32-53 output clear                   |
| GPIO_IN_REG        | 0x6000403c | GPIO 0-31 input register                  |
| GPIO_IN1_REG       | 0x60004040 | GPIO 32-53 input register                 |

## Direct ESP32-S3 GPIO Pin Assignments

Configuration array located at `0x3fc9dd54` (18 entries, 0x1c bytes each).

| GPIO | Signal Name | Direction | Function                | Notes                                  |
| ---- | ----------- | --------- | ----------------------- | -------------------------------------- |
| 4    | button      | Input     | User button             | Physical button on device              |
| 5    | chg_stat    | Input     | Charger status          | Battery charger IC status              |
| 6    | chg_pgood   | Input     | Charger power good      | Battery charger power good signal      |
| 7    | gauge_int   | Input     | Battery gauge interrupt | Fuel gauge IC interrupt                |
| 8    | tp_int      | Input     | Touch panel interrupt   | Touchscreen controller interrupt       |
| 17   | display_dc  | Output    | Display D/C             | Data/Command select for display        |
| 18   | qsfp_abs    | Input     | QSFP module absent      | QSFP cage presence detect (active low) |
| 21   | backlight   | Output    | Display backlight       | PWM-capable, controlled via LEDC       |
| 38   | sfp_pwr_en  | Output    | SFP power enable        | Controls power to SFP module           |
| 46   | sfp_pwr_en  | Output    | SFP power enable (alt)  | Alternative SFP power control          |
| 47   | ioe_int     | Input     | I/O expander interrupt  | CA9554 interrupt output                |
| 48   | display_rst | Output    | Display reset           | Display controller reset               |

### Unused/Reserved GPIOs

**GPIO0** is **NOT used** by the firmware for application functions. GPIO0 is an ESP32-S3 strapping pin that controls boot mode:

- LOW at reset: Download boot (UART flashing)
- HIGH at reset: Normal SPI flash boot

Using GPIO0 for application I/O would risk boot issues, so it's left unconnected or weakly pulled high.

## I/O Expander (CA9554) Pin Assignments

The SFP Wizard uses a **CA9554** (or compatible PCA9554) I2C GPIO expander for additional I/O. Configuration at `0x3fc9df94`.

I2C Address: Determined by hardware strapping (typically 0x20-0x27 range)

| IOE Pin | Signal Name | Direction | Function              | Notes                                 |
| ------- | ----------- | --------- | --------------------- | ------------------------------------- |
| P0      | tp_rst      | Output    | Touch panel reset     | Touchscreen controller reset          |
| P1      | (reserved)  | -         | -                     |                                       |
| P2      | qsfp_pwr_en | Output    | QSFP power enable     | Controls power to QSFP module         |
| P3      | sw_pwr_off  | Output    | Software power off    | System power control                  |
| P4      | chg_disable | Output    | Charger disable       | Disables battery charging             |
| P5      | (unknown)   | -         | -                     |                                       |
| P6      | xsfp_laser  | Output    | SFP/QSFP laser enable | Controls optical transmitter          |
| P7      | sfp_abs     | Input     | SFP module absent     | SFP cage presence detect (active low) |

## GPIO Functions (Firmware)

### Low-Level GPIO Functions

| Function         | Address    | Signature                                      | Description                            |
| ---------------- | ---------- | ---------------------------------------------- | -------------------------------------- |
| `gpio_set_level` | 0x420835fc | `int gpio_set_level(uint gpio_num, int level)` | Sets GPIO output level (0=low, 1=high) |
| `gpio_get_level` | 0x420836e0 | `uint gpio_get_level(uint gpio_num)`           | Reads GPIO input level, returns 0 or 1 |

### Higher-Level GPIO Functions

| Function                | Address    | Signature                                                         | Description                         |
| ----------------------- | ---------- | ----------------------------------------------------------------- | ----------------------------------- |
| `gpio_read_pin`         | 0x4206b430 | `uint gpio_read_pin(void *ctx, uint gpio_num)`                    | Reads GPIO with mode validation     |
| `gpio_write_pin`        | 0x4206b414 | `int gpio_write_pin(void *ctx, uint gpio_num, int level)`         | Writes GPIO with error handling     |
| `gpio_configure_output` | 0x4206b4d4 | `int gpio_configure_output(void *ctx, uint gpio_num, uint flags)` | Configures GPIO as output           |
| `gpio_find_pin_by_name` | 0x42013938 | `void * gpio_find_pin_by_name(char *name)`                        | Looks up GPIO config by signal name |

### Board Initialization Functions

| Function              | Address    | Description                                       |
| --------------------- | ---------- | ------------------------------------------------- |
| `board_init_early`    | 0x42013d80 | Main board early init, calls GPIO/I2C/IOE init    |
| `board_gpio_init`     | 0x4201385c | Initializes ESP-IDF GPIO driver                   |
| `board_gpio_pin_init` | 0x42013c74 | Configures individual GPIO pins from config array |
| `board_ioe_init`      | 0x42013cc4 | Initializes CA9554 I/O expander                   |

### Display Functions (using GPIO17 for D/C)

| Function                 | Address    | Description                        |
| ------------------------ | ---------- | ---------------------------------- |
| `display_write_cmd`      | 0x42036e1c | Write command to display (DC=high) |
| `display_write_data`     | 0x42036e00 | Write data to display (DC=low)     |
| `display_write_cmd_data` | 0x42036dd0 | Write command with data            |
| `disp_backlight_set`     | 0x42036d14 | Set backlight brightness (0-100%)  |

## GPIO Configuration Structure

Each GPIO pin config entry is 0x1c (28) bytes:

```c
struct gpio_pin_config {
    char *name;           // +0x00: Signal name string pointer
    uint32_t reserved[4]; // +0x04: Reserved/padding
    void *vtable;         // +0x14: Function pointer table
    uint32_t gpio_num;    // +0x18: GPIO pin number
};
```

## Data Pointers

| Symbol            | Address    | Value         | Description                    |
| ----------------- | ---------- | ------------- | ------------------------------ |
| GPIO_OUT_REG_PTR  | 0x42040ae8 | 0x60004004    | Pointer to GPIO_OUT_REG        |
| GPIO_OUT1_REG_PTR | 0x42040aec | 0x60004010    | Pointer to GPIO_OUT1_REG       |
| PTR_PTR_4207dd2c  | 0x4207dd2c | -> 0x3fca0248 | GPIO peripheral base structure |

## SFP Module Detection

SFP module presence is detected via the **sfp_abs** signal:

- **SFP**: Connected to I/O expander pin P7
- **QSFP**: Connected directly to ESP32 GPIO18

Both signals are active-low (low = module present, high = module absent).

When module presence changes, the firmware emits events:

- `sfp.plug.state.changed` (string at 0x3c1077b8)
- `qsfp.plug.state.changed` (string at 0x3c1077d0)

## Power Control

| Signal      | Location   | Function                     |
| ----------- | ---------- | ---------------------------- |
| sfp_pwr_en  | GPIO 38/46 | Enables power to SFP module  |
| qsfp_pwr_en | IOE P2     | Enables power to QSFP module |
| xsfp_laser  | IOE P6     | Enables laser transmitter    |
| sw_pwr_off  | IOE P3     | System power control         |
| chg_disable | IOE P4     | Disables battery charging    |
