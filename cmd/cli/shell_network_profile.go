package main

import (
	"fmt"
	"strings"
)

type geoLookupResult struct {
	Provider      string
	IP            string
	City          string
	Region        string
	Country       string
	CountryCode   string
	Latitude      float64
	Longitude     float64
	Timezone      string
	Org           string
	ISP           string
	VPN           bool
	Proxy         bool
	Tor           bool
	SecurityKnown bool
}

type vpnInspection struct {
	DefaultInterface string
	VPNInterfaces    []string
	VPNProcesses     []string
	ActiveVPNConns   []string
	GeoSignal        geoLookupResult
	LikelyActive     bool
}

type localNetworkTool struct {
	Name        string
	Commands    []string
	Description string
}

type webNetworkTool struct {
	Name        string
	URL         string
	Description string
}

func showNetworkProfile() (string, error) {
	location, _ := showNetworkLocation()
	vpn, _ := showVPNStatus()
	tools, _ := showNetworkToolsCatalog()
	ip, _ := showNetworkIP()

	lines := []string{
		"Network profile summary:",
	}
	if strings.TrimSpace(ip) != "" {
		lines = append(lines, ip)
	}
	if strings.TrimSpace(location) != "" {
		lines = append(lines, "")
		lines = append(lines, location)
	}
	if strings.TrimSpace(vpn) != "" {
		lines = append(lines, "")
		lines = append(lines, vpn)
	}
	if strings.TrimSpace(tools) != "" {
		lines = append(lines, "")
		lines = append(lines, tools)
	}
	return strings.Join(lines, "\n"), nil
}

func showNetworkLocation() (string, error) {
	publicIP := strings.TrimSpace(discoverPublicIPv4())
	if publicIP == "" {
		return "Network location snapshot:\npublic_ip=(unavailable)\nlocation=(unavailable)", nil
	}

	geo, err := lookupPublicIPLocation(publicIP)
	if err != nil {
		lines := []string{
			"Network location snapshot:",
			"public_ip=" + publicIP,
			"location=(lookup_failed)",
			"lookup_error=" + safeValue(strings.TrimSpace(err.Error()), "unknown error"),
		}
		return strings.Join(lines, "\n"), nil
	}

	locationParts := []string{}
	if geo.City != "" {
		locationParts = append(locationParts, geo.City)
	}
	if geo.Region != "" && geo.Region != geo.City {
		locationParts = append(locationParts, geo.Region)
	}
	if geo.Country != "" {
		locationParts = append(locationParts, geo.Country)
	}
	locationText := "(unknown)"
	if len(locationParts) > 0 {
		locationText = strings.Join(locationParts, ", ")
	}

	lines := []string{
		"Network location snapshot:",
		"provider=" + safeValue(geo.Provider, "unknown"),
		"public_ip=" + safeValue(geo.IP, publicIP),
		"location=" + locationText,
		"coordinates=" + fmt.Sprintf("%.4f,%.4f", geo.Latitude, geo.Longitude),
		"timezone=" + safeValue(geo.Timezone, "unknown"),
	}
	if geo.Org != "" {
		lines = append(lines, "network_org="+geo.Org)
	}
	if geo.ISP != "" {
		lines = append(lines, "isp="+geo.ISP)
	}
	if geo.SecurityKnown {
		lines = append(lines, fmt.Sprintf("security_signal=vpn:%t proxy:%t tor:%t", geo.VPN, geo.Proxy, geo.Tor))
	} else {
		lines = append(lines, "security_signal=(provider did not return vpn/proxy/tor flags)")
	}
	return strings.Join(lines, "\n"), nil
}

func showVPNStatus() (string, error) {
	inspection := inspectVPNState()

	lines := []string{
		"VPN inspection:",
		"default_route_interface=" + safeValue(inspection.DefaultInterface, "unknown"),
	}
	if len(inspection.VPNInterfaces) > 0 {
		lines = append(lines, "vpn_interfaces="+strings.Join(inspection.VPNInterfaces, ","))
	} else {
		lines = append(lines, "vpn_interfaces=(none detected)")
	}
	if len(inspection.ActiveVPNConns) > 0 {
		lines = append(lines, "active_vpn_connections="+strings.Join(inspection.ActiveVPNConns, ","))
	} else {
		lines = append(lines, "active_vpn_connections=(none detected)")
	}
	if len(inspection.VPNProcesses) > 0 {
		lines = append(lines, "vpn_processes="+strings.Join(inspection.VPNProcesses, " | "))
	} else {
		lines = append(lines, "vpn_processes=(none detected)")
	}
	if inspection.GeoSignal.SecurityKnown {
		lines = append(lines, fmt.Sprintf("geo_security_signal=vpn:%t proxy:%t tor:%t", inspection.GeoSignal.VPN, inspection.GeoSignal.Proxy, inspection.GeoSignal.Tor))
	}
	lines = append(lines, fmt.Sprintf("likely_vpn_active=%t", inspection.LikelyActive))
	return strings.Join(lines, "\n"), nil
}

func showNetworkToolsCatalog() (string, error) {
	available := []string{}
	missing := []string{}

	for _, tool := range knownLocalNetworkTools() {
		found := ""
		for _, cmd := range tool.Commands {
			if commandExists(cmd) {
				found = cmd
				break
			}
		}
		if found != "" {
			available = append(available, fmt.Sprintf("%s (via `%s`): %s", tool.Name, found, tool.Description))
		} else {
			missing = append(missing, fmt.Sprintf("%s (expects `%s`): %s", tool.Name, strings.Join(tool.Commands, "` or `"), tool.Description))
		}
	}

	lines := []string{
		"Network tools catalog:",
	}
	if len(available) > 0 {
		lines = append(lines, "local_tools_available:")
		for _, line := range available {
			lines = append(lines, "- "+line)
		}
	} else {
		lines = append(lines, "local_tools_available=(none detected)")
	}
	if len(missing) > 0 {
		lines = append(lines, "local_tools_missing:")
		for _, line := range missing {
			lines = append(lines, "- "+line)
		}
	}

	installCmd := inferNetworkToolsInstallCommand()
	if installCmd != "" {
		lines = append(lines, "recommended_install_command="+installCmd)
	}

	lines = append(lines, "web_tools_catalog:")
	for _, tool := range knownWebNetworkTools() {
		lines = append(lines, fmt.Sprintf("- %s: %s (%s)", tool.Name, tool.URL, tool.Description))
	}
	return strings.Join(lines, "\n"), nil
}

func knownLocalNetworkTools() []localNetworkTool {
	return []localNetworkTool{
		{Name: "Interface + route inspector", Commands: []string{"ip", "ifconfig"}, Description: "Inspect network interfaces, addresses, and default routes."},
		{Name: "Open ports inspector", Commands: []string{"ss", "netstat", "lsof"}, Description: "List listening TCP/UDP sockets and active port bindings."},
		{Name: "DNS lookup", Commands: []string{"dig", "nslookup", "host"}, Description: "Resolve domains, inspect DNS records, and troubleshoot DNS."},
		{Name: "Traceroute", Commands: []string{"traceroute", "mtr"}, Description: "Trace network path and identify latency hops."},
		{Name: "Network scanner", Commands: []string{"nmap"}, Description: "Probe hosts/ports for diagnostics and service discovery."},
		{Name: "VPN status", Commands: []string{"nmcli", "wg", "openvpn"}, Description: "Inspect VPN clients/connections and tunnel state."},
		{Name: "WHOIS", Commands: []string{"whois"}, Description: "Inspect domain/IP registration metadata."},
	}
}

func knownWebNetworkTools() []webNetworkTool {
	return []webNetworkTool{
		{Name: "IPify", URL: "https://api.ipify.org", Description: "Returns your current public IP address."},
		{Name: "IPWho.is", URL: "https://ipwho.is", Description: "Public IP geolocation + ISP/org + optional VPN/proxy/Tor flags."},
		{Name: "IPAPI", URL: "https://ipapi.co/json", Description: "Public IP geolocation and connection metadata fallback."},
		{Name: "BGP.he.net", URL: "https://bgp.he.net", Description: "ASN, prefixes, and routing context for IP/network operators."},
		{Name: "Shodan", URL: "https://www.shodan.io", Description: "Internet-exposed service discovery and host intelligence."},
		{Name: "Censys", URL: "https://search.censys.io", Description: "Internet-wide host/certificate search for security research."},
		{Name: "VirusTotal URL/IP", URL: "https://www.virustotal.com", Description: "Reputation and threat checks for URLs/domains/IPs."},
		{Name: "WhatIsMyDNS", URL: "https://www.whatsmydns.net", Description: "DNS propagation checks across global resolvers."},
	}
}

func inferNetworkToolsInstallCommand() string {
	if fileExists("./scripts/setup-host-deps.sh") {
		return "./scripts/setup-host-deps.sh --profile local -y"
	}
	if fileExists("scripts/setup-host-deps.sh") {
		return "scripts/setup-host-deps.sh --profile local -y"
	}
	return ""
}
