package blacklist

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
)

type Blacklist struct {
	IPs  []string `json:"ips"`
	mu   sync.RWMutex
	file string
}

func NewBlacklist(file string) (*Blacklist, error) {
	bl := &Blacklist{
		IPs:  []string{},
		file: file,
	}
	err := bl.Load()
	if err != nil {
		if os.IsNotExist(err) {
			// If the file doesn't exist, create it with an empty blacklist
			return bl, bl.Save()
		}
		return nil, err
	}
	return bl, nil
}

func (bl *Blacklist) Load() error {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	data, err := os.ReadFile(bl.file)
	if err != nil {
		return err
	}

	var temp struct {
		IPs []string `json:"ips"`
	}

	err = json.Unmarshal(data, &temp)
	if err != nil {
		return err
	}

	bl.IPs = temp.IPs
	return nil
}

func (bl *Blacklist) Save() error {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	temp := struct {
		IPs []string `json:"ips"`
	}{
		IPs: bl.IPs,
	}

	data, err := json.MarshalIndent(temp, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(bl.file, data, 0644)
}

func (bl *Blacklist) Add(ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	for _, existingIP := range bl.IPs {
		if existingIP == ip {
			return nil // IP already in the list
		}
	}

	bl.IPs = append(bl.IPs, ip)

	temp := struct {
		IPs []string `json:"ips"`
	}{
		IPs: bl.IPs,
	}

	data, err := json.MarshalIndent(temp, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(bl.file, data, 0644)
}

func (bl *Blacklist) Remove(ip string) error {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	for i, existingIP := range bl.IPs {
		if existingIP == ip {
			bl.IPs = append(bl.IPs[:i], bl.IPs[i+1:]...)
			
			temp := struct {
				IPs []string `json:"ips"`
			}{
				IPs: bl.IPs,
			}

			data, err := json.MarshalIndent(temp, "", "  ")
			if err != nil {
				return err
			}

			return os.WriteFile(bl.file, data, 0644)
		}
	}

	return nil // IP not found in the list
}

func (bl *Blacklist) Contains(ip string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	for _, existingIP := range bl.IPs {
		if existingIP == ip {
			return true
		}
	}
	return false
}

func (bl *Blacklist) GetIPs() []string {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	return append([]string{}, bl.IPs...)
}

