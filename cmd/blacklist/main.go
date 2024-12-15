package main

import (
	"flag"
	"fmt"
	"os"

	"go.minekube.com/gate/pkg/edition/java/lite/blacklist"
)

func main() {
	blacklistFile := flag.String("file", "ip_blacklist.json", "Path to the global blacklist JSON file")
	routeBlacklistFile := flag.String("route-file", "route_blacklist.json", "Path to the route blacklist JSON file")
	add := flag.String("add", "", "IP address to add to the blacklist")
	remove := flag.String("remove", "", "IP address to remove from the blacklist")
	route := flag.String("route", "", "Route for adding/removing IP (if not specified, uses global blacklist)")
	list := flag.Bool("list", false, "List all blacklisted IPs")
	flag.Parse()

	bl, err := blacklist.NewBlacklist(*blacklistFile)
	if err != nil {
		fmt.Printf("Error initializing global blacklist: %v\n", err)
		os.Exit(1)
	}

	rbl, err := blacklist.NewRouteBlacklist(*routeBlacklistFile)
	if err != nil {
		fmt.Printf("Error initializing route blacklist: %v\n", err)
		os.Exit(1)
	}

	if *add != "" {
		if *route != "" {
			err = rbl.Add(*route, *add)
		} else {
			err = bl.Add(*add)
		}
		if err != nil {
			fmt.Printf("Error adding IP to blacklist: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added %s to the blacklist\n", *add)
	}

	if *remove != "" {
		if *route != "" {
			err = rbl.Remove(*route, *remove)
		} else {
			err = bl.Remove(*remove)
		}
		if err != nil {
			fmt.Printf("Error removing IP from blacklist: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed %s from the blacklist\n", *remove)
	}

	if *list {
		fmt.Println("Global Blacklisted IPs:")
		for _, ip := range bl.GetIPs() {
			fmt.Println(ip)
		}
		fmt.Println("\nRoute Blacklisted IPs:")
		for route, ips := range rbl.Blacklists {
			fmt.Printf("Route: %s\n", route)
			for _, ip := range ips {
				fmt.Printf("  %s\n", ip)
			}
		}
	}
}

