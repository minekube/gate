package blacklist

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
)

type RouteBlacklist struct {
	Blacklists map[string][]string `json:"blacklists"`
	mu         sync.RWMutex
	file       string
}

func NewRouteBlacklist(file string) (*RouteBlacklist, error) {
	rb := &RouteBlacklist{
		Blacklists: make(map[string][]string),
		file:       file,
	}
	err := rb.Load()
	if err != nil {
		if os.IsNotExist(err) {
			return rb, rb.Save()
		}
		return nil, err
	}
	return rb, nil
}

func (rb *RouteBlacklist) Load() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	data, err := os.ReadFile(rb.file)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &rb.Blacklists)
}

func (rb *RouteBlacklist) Save() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	data, err := json.MarshalIndent(rb.Blacklists, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(rb.file, data, 0644)
}

func (rb *RouteBlacklist) Add(route, ip string) error {
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	if _, ok := rb.Blacklists[route]; !ok {
		rb.Blacklists[route] = []string{}
	}

	for _, existingIP := range rb.Blacklists[route] {
		if existingIP == ip {
			return nil // IP already in the list
		}
	}

	rb.Blacklists[route] = append(rb.Blacklists[route], ip)
	return rb.Save()
}

func (rb *RouteBlacklist) Remove(route, ip string) error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if ips, ok := rb.Blacklists[route]; ok {
		for i, existingIP := range ips {
			if existingIP == ip {
				rb.Blacklists[route] = append(ips[:i], ips[i+1:]...)
				return rb.Save()
			}
		}
	}

	return nil // IP not found in the list
}

func (rb *RouteBlacklist) Contains(route, ip string) bool {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if ips, ok := rb.Blacklists[route]; ok {
		for _, existingIP := range ips {
			if existingIP == ip {
				return true
			}
		}
	}
	return false
}

func (rb *RouteBlacklist) GetIPs(route string) []string {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if ips, ok := rb.Blacklists[route]; ok {
		return append([]string{}, ips...)
	}
	return []string{}
}

