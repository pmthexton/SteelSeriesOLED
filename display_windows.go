package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type display struct {
	handle syscall.Handle
	path   string
}

func (d *display) connected() bool { return d.handle != 0 && d.handle != syscall.InvalidHandle }
func (d *display) refresh() error {
	devices, err := enumerate()
	if err != nil {
		return err
	}
	for _, dev := range devices {
		if !dev.OLED {
			continue
		}
		if d.connected() && d.path == dev.Path {
			return nil
		}
		d.close(false)
		h, e := open(dev.Path, syscall.GENERIC_READ|syscall.GENERIC_WRITE)
		if e != nil {
			return e
		}
		d.handle, d.path = h, dev.Path
		return nil
	}
	d.close(false)
	return nil
}
func (d *display) write(b Bitmap) error {
	if !d.connected() {
		return fmt.Errorf("OLED disconnected")
	}
	for _, report := range b.Reports() {
		ok, _, err := hid.NewProc("HidD_SetFeature").Call(uintptr(d.handle), uintptr(unsafe.Pointer(&report[0])), uintptr(len(report)))
		if ok == 0 {
			d.close(false)
			return fmt.Errorf("OLED feature report: %w", err)
		}
	}
	return nil
}
func (d *display) close(restore bool) {
	if !d.connected() {
		return
	}
	if restore {
		var report [64]byte
		report[0], report[1] = 6, 0x95
		hid.NewProc("HidD_SetOutputReport").Call(uintptr(d.handle), uintptr(unsafe.Pointer(&report[0])), 64)
	}
	syscall.CloseHandle(d.handle)
	d.handle = 0
	d.path = ""
}
