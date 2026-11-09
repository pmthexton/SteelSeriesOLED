// Throwaway Windows HID visibility probe. Does not send any USB reports.
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	hid           = syscall.NewLazyDLL("hid.dll")
	setup         = syscall.NewLazyDLL("setupapi.dll")
	getGUID       = hid.NewProc("HidD_GetHidGuid")
	getAttrs      = hid.NewProc("HidD_GetAttributes")
	getPreparsed  = hid.NewProc("HidD_GetPreparsedData")
	freePreparsed = hid.NewProc("HidD_FreePreparsedData")
	getCaps       = hid.NewProc("HidP_GetCaps")
	getClass      = setup.NewProc("SetupDiGetClassDevsW")
	enumInterface = setup.NewProc("SetupDiEnumDeviceInterfaces")
	getDetail     = setup.NewProc("SetupDiGetDeviceInterfaceDetailW")
	destroy       = setup.NewProc("SetupDiDestroyDeviceInfoList")
)

type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}
type interfaceData struct {
	Size     uint32
	Class    guid
	Flags    uint32
	Reserved uintptr
}
type attributes struct {
	Size                     uint32
	Vendor, Product, Version uint16
}
type caps struct {
	Usage, UsagePage, Input, Output, Feature uint16
	Reserved                                 [17]uint16
	Counts                                   [10]uint16
}
type device struct {
	Path         string `json:"path"`
	Vendor       string `json:"vendor"`
	Product      string `json:"product"`
	UsagePage    string `json:"usage_page,omitempty"`
	FeatureBytes uint16 `json:"feature_bytes"`
	OutputBytes  uint16 `json:"output_bytes"`
	OLED         bool   `json:"matches_mac_oled"`
	ReadWrite    bool   `json:"read_write_open"`
	Error        string `json:"error,omitempty"`
}

func open(path string, access uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	return syscall.CreateFile(p, access, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
}

func inspect(path string) device {
	d := device{Path: path}
	h, err := open(path, 0)
	if err != nil {
		d.Error = "metadata open: " + err.Error()
		return d
	}
	defer syscall.CloseHandle(h)
	a := attributes{}
	a.Size = uint32(unsafe.Sizeof(a))
	if ok, _, e := getAttrs.Call(uintptr(h), uintptr(unsafe.Pointer(&a))); ok == 0 {
		d.Error = "attributes: " + e.Error()
		return d
	}
	d.Vendor, d.Product = fmt.Sprintf("%04x", a.Vendor), fmt.Sprintf("%04x", a.Product)
	var pp uintptr
	if ok, _, e := getPreparsed.Call(uintptr(h), uintptr(unsafe.Pointer(&pp))); ok == 0 {
		d.Error = "preparsed data: " + e.Error()
		return d
	}
	defer freePreparsed.Call(pp)
	var c caps
	if status, _, _ := getCaps.Call(pp, uintptr(unsafe.Pointer(&c))); uint32(status) != 0x00110000 {
		d.Error = fmt.Sprintf("HidP_GetCaps: 0x%x", status)
		return d
	}
	d.UsagePage = fmt.Sprintf("%04x", c.UsagePage)
	d.FeatureBytes, d.OutputBytes = c.Feature, c.Output
	d.OLED = a.Vendor == 0x1038 && a.Product == 0x12e0 && c.UsagePage == 0xffc0 && c.Feature >= 1024
	if d.OLED {
		rw, err := open(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE)
		if err != nil {
			d.Error = "read/write open: " + err.Error()
		} else {
			d.ReadWrite = true
			syscall.CloseHandle(rw)
		}
	}
	return d
}

func enumerate() ([]device, error) {
	var g guid
	getGUID.Call(uintptr(unsafe.Pointer(&g)))
	set, _, e := getClass.Call(uintptr(unsafe.Pointer(&g)), 0, 0, 0x12) // PRESENT | DEVICEINTERFACE
	if set == ^uintptr(0) {
		return nil, fmt.Errorf("SetupDiGetClassDevs: %w", e)
	}
	defer destroy.Call(set)
	result := []device{}
	for i := uint32(0); ; i++ {
		item := interfaceData{}
		item.Size = uint32(unsafe.Sizeof(item))
		ok, _, e := enumInterface.Call(set, 0, uintptr(unsafe.Pointer(&g)), uintptr(i), uintptr(unsafe.Pointer(&item)))
		if ok == 0 {
			if e == syscall.Errno(259) {
				break
			}
			return nil, fmt.Errorf("SetupDiEnumDeviceInterfaces: %w", e)
		}
		var size uint32
		ok, _, e = getDetail.Call(set, uintptr(unsafe.Pointer(&item)), 0, 0, uintptr(unsafe.Pointer(&size)), 0)
		if ok == 0 && e != syscall.ERROR_INSUFFICIENT_BUFFER {
			return nil, fmt.Errorf("interface size: %w", e)
		}
		if size < 6 {
			return nil, fmt.Errorf("invalid interface detail size %d", size)
		}
		buf := make([]byte, size)
		cbSize := uint32(8)
		if unsafe.Sizeof(uintptr(0)) == 4 {
			cbSize = 6
		}
		binary.LittleEndian.PutUint32(buf, cbSize)
		ok, _, e = getDetail.Call(set, uintptr(unsafe.Pointer(&item)), uintptr(unsafe.Pointer(&buf[0])), uintptr(size), 0, 0)
		if ok == 0 {
			return nil, fmt.Errorf("interface detail: %w", e)
		}
		// DevicePath starts at byte 4, including on 64-bit Windows.
		units := make([]uint16, 0, (len(buf)-4)/2)
		for n := 4; n+1 < len(buf); n += 2 {
			v := binary.LittleEndian.Uint16(buf[n:])
			if v == 0 {
				break
			}
			units = append(units, v)
		}
		path := syscall.UTF16ToString(units)
		if strings.Contains(strings.ToLower(path), "vid_1038") {
			result = append(result, inspect(path))
		}
	}
	return result, nil
}

func main() {
	duration := flag.Duration("duration", 0, "exit after this duration (0 runs until Ctrl+C)")
	interval := flag.Duration("interval", 500*time.Millisecond, "poll interval")
	flag.Parse()
	if *interval < 100*time.Millisecond || *duration < 0 {
		fmt.Fprintln(os.Stderr, "interval must be >=100ms; duration must be >=0")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "Read-only SteelSeries probe: switch Windows -> Mac -> Windows. Presence is NOT yet proof of active source. No reports are sent.")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	var timeout <-chan time.Time
	if *duration > 0 {
		t := time.NewTimer(*duration)
		defer t.Stop()
		timeout = t.C
	}
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	previous := ""
	enc := json.NewEncoder(os.Stdout)
	for {
		devices, err := enumerate()
		state := "oled_absent"
		errText := ""
		if err != nil {
			state = "enumeration_error"
			errText = err.Error()
		} else {
			for _, d := range devices {
				if d.OLED {
					state = "oled_present"
					break
				}
				if d.Error != "" {
					state = "steelseries_unreadable"
				}
			}
		}
		key, _ := json.Marshal(struct {
			State   string
			Devices []device
			Error   string
		}{state, devices, errText})
		if string(key) != previous {
			if err := enc.Encode(struct {
				Time    string   `json:"time"`
				State   string   `json:"state"`
				Devices []device `json:"devices"`
				Error   string   `json:"error,omitempty"`
			}{time.Now().Format(time.RFC3339Nano), state, devices, errText}); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			previous = string(key)
		}
		select {
		case <-signals:
			return
		case <-timeout:
			return
		case <-ticker.C:
		}
	}
}
