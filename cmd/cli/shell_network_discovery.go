package main

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func inspectVPNState() vpnInspection {
	inspection := vpnInspection{
		DefaultInterface: discoverDefaultRouteInterface(),
		VPNInterfaces:    discoverVPNInterfaces(),
		VPNProcesses:     discoverVPNProcesses(),
		ActiveVPNConns:   discoverActiveVPNConnectionsNmcli(),
	}

	publicIP := discoverPublicIPv4()
	if publicIP != "" {
		if geo, err := lookupPublicIPLocation(publicIP); err == nil {
			inspection.GeoSignal = geo
		}
	}

	inspection.LikelyActive = len(inspection.VPNInterfaces) > 0 ||
		len(inspection.VPNProcesses) > 0 ||
		len(inspection.ActiveVPNConns) > 0 ||
		(inspection.GeoSignal.SecurityKnown && (inspection.GeoSignal.VPN || inspection.GeoSignal.Proxy || inspection.GeoSignal.Tor))
	return inspection
}

func discoverDefaultRouteInterface() string {
	if !commandExists("ip") {
		return ""
	}
	raw, err := runLocalCommand([]string{"ip", "route", "show", "default"}, localShellCommandTimeout)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] == "dev" {
				return strings.TrimSpace(fields[i+1])
			}
		}
	}
	return ""
}

func discoverVPNInterfaces() []string {
	found := []string{}
	seen := map[string]struct{}{}

	if commandExists("ip") {
		if raw, err := runLocalCommand([]string{"ip", "-o", "link", "show"}, localShellCommandTimeout); err == nil {
			for _, line := range strings.Split(raw, "\n") {
				parts := strings.SplitN(line, ":", 3)
				if len(parts) < 2 {
					continue
				}
				iface := strings.TrimSpace(parts[1])
				if idx := strings.Index(iface, "@"); idx > 0 {
					iface = iface[:idx]
				}
				lower := strings.ToLower(strings.TrimSpace(iface))
				if lower == "" || !networkInterfaceNamePattern.MatchString(lower) {
					continue
				}
				if _, ok := seen[lower]; ok {
					continue
				}
				seen[lower] = struct{}{}
				found = append(found, lower)
			}
		}
	}

	if len(found) == 0 && commandExists("ifconfig") {
		if raw, err := runLocalCommand([]string{"ifconfig", "-a"}, localShellCommandTimeout); err == nil {
			for _, line := range strings.Split(raw, "\n") {
				line = strings.TrimSpace(line)
				matches := ifconfigInterfacePattern.FindStringSubmatch(line)
				if len(matches) != 2 {
					continue
				}
				lower := strings.ToLower(strings.TrimSpace(matches[1]))
				if lower == "" || !networkInterfaceNamePattern.MatchString(lower) {
					continue
				}
				if _, ok := seen[lower]; ok {
					continue
				}
				seen[lower] = struct{}{}
				found = append(found, lower)
			}
		}
	}

	return found
}

func discoverVPNProcesses() []string {
	if !commandExists("pgrep") {
		return nil
	}
	patterns := []string{"openvpn", "wg-quick", "wireguard", "tailscaled", "protonvpn", "nordvpn", "mullvad", "openconnect", "pia", "expressvpn", "ivpn"}
	found := []string{}
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		raw, err := runLocalCommand([]string{"pgrep", "-af", pattern}, 4*time.Second)
		if err != nil || strings.TrimSpace(raw) == "" {
			continue
		}
		for _, line := range strings.Split(raw, "\n") {
			entry := trimLocalText(strings.TrimSpace(line), 180)
			if entry == "" {
				continue
			}
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			found = append(found, entry)
		}
	}
	return found
}

func discoverActiveVPNConnectionsNmcli() []string {
	if !commandExists("nmcli") {
		return nil
	}
	raw, err := runLocalCommand([]string{"nmcli", "-t", "-f", "NAME,TYPE,DEVICE", "connection", "show", "--active"}, localShellCommandTimeout)
	if err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		if strings.ToLower(strings.TrimSpace(parts[1])) != "vpn" {
			continue
		}
		name := strings.TrimSpace(parts[0])
		device := ""
		if len(parts) > 2 {
			device = strings.TrimSpace(parts[2])
		}
		entry := name
		if device != "" {
			entry = name + " (device=" + device + ")"
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func lookupPublicIPLocation(ip string) (geoLookupResult, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return geoLookupResult{}, errors.New("public ip is required for location lookup")
	}

	if geo, err := lookupGeoViaIPWhoIs(ip); err == nil {
		return geo, nil
	}
	return lookupGeoViaIPAPI()
}

func lookupGeoViaIPWhoIs(ip string) (geoLookupResult, error) {
	raw, err := fetchWebText("https://ipwho.is/" + ip)
	if err != nil {
		return geoLookupResult{}, err
	}

	var payload struct {
		Success     bool    `json:"success"`
		Message     string  `json:"message"`
		IP          string  `json:"ip"`
		City        string  `json:"city"`
		Region      string  `json:"region"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Timezone    struct {
			ID string `json:"id"`
		} `json:"timezone"`
		Connection struct {
			ISP string `json:"isp"`
			Org string `json:"org"`
		} `json:"connection"`
		Security struct {
			VPN   bool `json:"vpn"`
			Proxy bool `json:"proxy"`
			Tor   bool `json:"tor"`
		} `json:"security"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return geoLookupResult{}, err
	}
	if !payload.Success {
		return geoLookupResult{}, errors.New(safeValue(strings.TrimSpace(payload.Message), "ipwho.is returned unsuccessful status"))
	}

	return geoLookupResult{
		Provider:      "ipwho.is",
		IP:            strings.TrimSpace(payload.IP),
		City:          strings.TrimSpace(payload.City),
		Region:        strings.TrimSpace(payload.Region),
		Country:       strings.TrimSpace(payload.Country),
		CountryCode:   strings.TrimSpace(payload.CountryCode),
		Latitude:      payload.Latitude,
		Longitude:     payload.Longitude,
		Timezone:      strings.TrimSpace(payload.Timezone.ID),
		Org:           strings.TrimSpace(payload.Connection.Org),
		ISP:           strings.TrimSpace(payload.Connection.ISP),
		VPN:           payload.Security.VPN,
		Proxy:         payload.Security.Proxy,
		Tor:           payload.Security.Tor,
		SecurityKnown: true,
	}, nil
}

func lookupGeoViaIPAPI() (geoLookupResult, error) {
	raw, err := fetchWebText("https://ipapi.co/json/")
	if err != nil {
		return geoLookupResult{}, err
	}

	var payload struct {
		IP          string  `json:"ip"`
		City        string  `json:"city"`
		Region      string  `json:"region"`
		CountryName string  `json:"country_name"`
		CountryCode string  `json:"country_code"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Timezone    string  `json:"timezone"`
		Org         string  `json:"org"`
		Error       bool    `json:"error"`
		Reason      string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return geoLookupResult{}, err
	}
	if payload.Error {
		return geoLookupResult{}, errors.New(safeValue(strings.TrimSpace(payload.Reason), "ipapi returned error"))
	}

	return geoLookupResult{
		Provider:      "ipapi.co",
		IP:            strings.TrimSpace(payload.IP),
		City:          strings.TrimSpace(payload.City),
		Region:        strings.TrimSpace(payload.Region),
		Country:       strings.TrimSpace(payload.CountryName),
		CountryCode:   strings.TrimSpace(payload.CountryCode),
		Latitude:      payload.Latitude,
		Longitude:     payload.Longitude,
		Timezone:      strings.TrimSpace(payload.Timezone),
		Org:           strings.TrimSpace(payload.Org),
		SecurityKnown: false,
	}, nil
}
