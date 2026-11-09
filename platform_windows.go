package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"syscall"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var queryPath = kernel.NewProc("QueryFullProcessImageNameW")
var foreground = syscall.NewLazyDLL("user32.dll").NewProc("GetForegroundWindow")
var windowPID = syscall.NewLazyDLL("user32.dll").NewProc("GetWindowThreadProcessId")

type Game struct {
	PID  uint32
	Path string
}

func captureTargetChanged(current, next Game, byName bool) bool {
	if (current.PID == 0) != (next.PID == 0) {
		return true
	}
	if byName && current.PID != 0 {
		return !stringsEqual(current.Path, next.Path)
	}
	return current.PID != next.PID
}
func stringsEqual(a, b string) bool { return normalizedPath(a) == normalizedPath(b) }

func steamPath() string {
	keyPath, _ := syscall.UTF16PtrFromString(`Software\Valve\Steam`)
	var key syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, keyPath, 0, syscall.KEY_READ, &key) != nil {
		return ""
	}
	defer syscall.RegCloseKey(key)
	name, _ := syscall.UTF16PtrFromString("SteamPath")
	buf := make([]uint16, 1024)
	n := uint32(len(buf) * 2)
	if syscall.RegQueryValueEx(key, name, nil, nil, (*byte)(unsafe.Pointer(&buf[0])), &n) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func gogGameRoots() []string {
	var roots []string
	for _, hive := range []syscall.Handle{syscall.HKEY_LOCAL_MACHINE, syscall.HKEY_CURRENT_USER} {
		for _, keyName := range []string{`SOFTWARE\GOG.com\Games`, `SOFTWARE\WOW6432Node\GOG.com\Games`} {
			path, _ := syscall.UTF16PtrFromString(keyName)
			var key syscall.Handle
			if syscall.RegOpenKeyEx(hive, path, 0, syscall.KEY_READ, &key) != nil {
				continue
			}
			for i := uint32(0); ; i++ {
				name := make([]uint16, 512)
				n := uint32(len(name))
				status, _, _ := syscall.NewLazyDLL("advapi32.dll").NewProc("RegEnumKeyExW").Call(uintptr(key), uintptr(i), uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&n)), 0, 0, 0, 0)
				if status == 259 {
					break
				}
				if status != 0 {
					break
				}
				subName, _ := syscall.UTF16PtrFromString(syscall.UTF16ToString(name[:n]))
				var sub syscall.Handle
				if syscall.RegOpenKeyEx(key, subName, 0, syscall.KEY_READ, &sub) != nil {
					continue
				}
				value, _ := syscall.UTF16PtrFromString("path")
				buf := make([]uint16, 32768)
				bytes := uint32(len(buf) * 2)
				if syscall.RegQueryValueEx(sub, value, nil, nil, (*byte)(unsafe.Pointer(&buf[0])), &bytes) == nil {
					installed := syscall.UTF16ToString(buf)
					if info, e := os.Stat(installed); e == nil && info.IsDir() {
						roots = append(roots, installed)
					}
				}
				syscall.RegCloseKey(sub)
			}
			syscall.RegCloseKey(key)
		}
	}
	return roots
}
func runningGames(c Config) ([]Game, error) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(2, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(snapshot)
	entry := syscall.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	err = syscall.Process32First(snapshot, &entry)
	games := []Game{}
	for err == nil {
		h, e := syscall.OpenProcess(0x1000, false, entry.ProcessID)
		if e == nil {
			buf := make([]uint16, 32768)
			size := uint32(len(buf))
			ok, _, _ := queryPath.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
			syscall.CloseHandle(h)
			if ok != 0 {
				path := syscall.UTF16ToString(buf[:size])
				if c.IsGame(path) {
					games = append(games, Game{entry.ProcessID, path})
				}
			}
		}
		err = syscall.Process32Next(snapshot, &entry)
	}
	if err != syscall.ERROR_NO_MORE_FILES {
		return nil, err
	}
	sort.Slice(games, func(i, j int) bool { return games[i].PID < games[j].PID })
	return games, nil
}
func chooseGame(games []Game, current uint32) Game {
	hwnd, _, _ := foreground.Call()
	var pid uint32
	windowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	for _, g := range games {
		if g.PID == pid {
			return g
		}
	}
	for _, g := range games {
		if g.PID == current {
			return g
		}
	}
	if len(games) > 0 {
		return games[0]
	}
	return Game{}
}

type cpuSampler struct {
	idle, kernel, user uint64
	ready              bool
}

func filetime(ft syscall.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}
func (s *cpuSampler) sample() float64 {
	var idle, k, u syscall.Filetime
	ok, _, _ := kernel.NewProc("GetSystemTimes").Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&k)), uintptr(unsafe.Pointer(&u)))
	if ok == 0 {
		return math.NaN()
	}
	a, b, c := filetime(idle), filetime(k), filetime(u)
	result := math.NaN()
	if s.ready && b >= s.kernel && c >= s.user && a >= s.idle {
		total := b - s.kernel + c - s.user
		idleDelta := a - s.idle
		if total > 0 && idleDelta <= total {
			result = 100 * (1 - float64(idleDelta)/float64(total))
		}
	}
	s.idle, s.kernel, s.user, s.ready = a, b, c, true
	return result
}

type gpuSampler struct {
	dll    *syscall.LazyDLL
	handle uintptr
}

func newGPU(index uint32) (*gpuSampler, error) {
	// Absolute Windows system path prevents loading a DLL from the working directory.
	dll := syscall.NewLazyDLL(os.Getenv("WINDIR") + `\System32\nvml.dll`)
	if e := dll.Load(); e != nil {
		return nil, e
	}
	for _, name := range []string{"nvmlInit_v2", "nvmlDeviceGetHandleByIndex_v2", "nvmlDeviceGetUtilizationRates", "nvmlDeviceGetTemperature", "nvmlShutdown"} {
		if e := dll.NewProc(name).Find(); e != nil {
			return nil, e
		}
	}
	status, _, _ := dll.NewProc("nvmlInit_v2").Call()
	if status != 0 {
		return nil, fmt.Errorf("NVML init: %d", status)
	}
	g := &gpuSampler{dll: dll}
	status, _, _ = dll.NewProc("nvmlDeviceGetHandleByIndex_v2").Call(uintptr(index), uintptr(unsafe.Pointer(&g.handle)))
	if status != 0 {
		g.close()
		return nil, fmt.Errorf("NVML device: %d", status)
	}
	return g, nil
}
func (g *gpuSampler) sample() (float64, float64) {
	usage, temp := math.NaN(), math.NaN()
	if g == nil {
		return usage, temp
	}
	var rates struct{ GPU, Memory uint32 }
	var t uint32
	if s, _, _ := g.dll.NewProc("nvmlDeviceGetUtilizationRates").Call(g.handle, uintptr(unsafe.Pointer(&rates))); s == 0 {
		usage = float64(rates.GPU)
	}
	if s, _, _ := g.dll.NewProc("nvmlDeviceGetTemperature").Call(g.handle, 0, uintptr(unsafe.Pointer(&t))); s == 0 {
		temp = float64(t)
	}
	return usage, temp
}
func (g *gpuSampler) close() {
	if g != nil {
		g.dll.NewProc("nvmlShutdown").Call()
	}
}

// A named mutex prevents duplicate login/manual instances from competing for OLED.
func singleInstance() (func(), error) {
	name, _ := syscall.UTF16PtrFromString(`Local\SteelSeriesOLED-` + os.Getenv("USERNAME"))
	h, _, err := kernel.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return nil, err
	}
	if err == syscall.Errno(183) {
		syscall.CloseHandle(syscall.Handle(h))
		return nil, fmt.Errorf("SteelSeriesOLED is already running")
	}
	return func() { syscall.CloseHandle(syscall.Handle(h)) }, nil
}
