/*
 * SPDX-License-Identifier: GPL-3.0
 * Vencord Installer, a cross platform gui/cli app for installing Vencord
 * Copyright (c) 2023 Vendicated and Vencord contributors
 */

//go:build linux || windows

package main

import (
	"io/fs"
	"os"
	path "path/filepath"
	"runtime"
	"strings"
)

func installerHomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// findResourcesAppAsarFiles walks searchRoots up to maxDepth directory levels and returns
// paths to resources/app.asar or resources/_app.asar.
func findResourcesAppAsarFiles(searchRoots []string, maxDepth int) []string {
	var out []string
	for _, root := range searchRoots {
		root = path.Clean(root)
		st, err := os.Stat(root)
		if err != nil || !st.IsDir() {
			continue
		}
		_ = path.WalkDir(root, func(walkPath string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, err := path.Rel(root, walkPath)
			if err != nil || rel == "." {
				return nil
			}
			depth := strings.Count(rel, string(os.PathSeparator))
			if d.IsDir() {
				if depth >= maxDepth {
					return path.SkipDir
				}
				switch strings.ToLower(d.Name()) {
				case "node_modules", ".git", "cache", "caches", "steamapps":
					return path.SkipDir
				}
				return nil
			}
			name := d.Name()
			if name != "app.asar" && name != "_app.asar" {
				return nil
			}
			if path.Base(path.Dir(walkPath)) != "resources" {
				return nil
			}
			out = append(out, walkPath)
			return nil
		})
	}
	return out
}

// discordInstallFromResourcesAsar maps a discovered asar path to a DiscordInstall using
// OS-specific layout: Windows expects ParseDiscord(parentOfAppVersionDir), Linux the app-* bundle dir.
func discordInstallFromResourcesAsar(asarPath string) *DiscordInstall {
	resDir := path.Dir(asarPath)
	if path.Base(resDir) != "resources" {
		return nil
	}
	bundleDir := path.Dir(resDir)
	base := path.Base(bundleDir)
	if !strings.HasPrefix(base, "app-") {
		return nil
	}

	switch runtime.GOOS {
	case "windows":
		parent := path.Dir(bundleDir)
		return ParseDiscord(parent, "")
	case "linux":
		return ParseDiscord(bundleDir, "")
	default:
		return nil
	}
}

func discoverDiscordInstallsFromAppAsarScan() []*DiscordInstall {
	var roots []string
	maxDepth := 6
	switch runtime.GOOS {
	case "windows":
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			roots = append(roots, la)
			maxDepth = 10
		}
	case "linux":
		for _, r := range []string{"/opt", "/usr/local"} {
			if ExistsFile(r) {
				roots = append(roots, r)
			}
		}
		home := installerHomeDir()
		if entries, err := os.ReadDir(path.Join(home, ".local/share")); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				n := strings.ToLower(e.Name())
				if strings.Contains(n, "discord") {
					roots = append(roots, path.Join(home, ".local/share", e.Name()))
				}
			}
		}
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = path.Join(home, ".config")
		}
		for _, e := range []string{
			"discord", "discordcanary", "DiscordCanary", "discordptb", "DiscordPTB",
			"discorddevelopment", "DiscordDevelopment",
		} {
			p := path.Join(configHome, e)
			if ExistsFile(p) {
				roots = append(roots, p)
			}
		}
		maxDepth = 8
	default:
		return nil
	}

	seen := make(map[string]struct{})
	var installs []*DiscordInstall
	for _, asar := range findResourcesAppAsarFiles(roots, maxDepth) {
		di := discordInstallFromResourcesAsar(asar)
		if di == nil {
			continue
		}
		if _, ok := seen[di.path]; ok {
			continue
		}
		seen[di.path] = struct{}{}
		installs = append(installs, di)
	}
	return installs
}

func mergeDiscordInstallsUnique(existing []any, extra []*DiscordInstall) []any {
	seen := make(map[string]struct{})
	var out []any
	add := func(d *DiscordInstall) {
		if d == nil {
			return
		}
		if _, ok := seen[d.path]; ok {
			return
		}
		seen[d.path] = struct{}{}
		out = append(out, d)
	}
	for _, x := range existing {
		if di, ok := x.(*DiscordInstall); ok {
			add(di)
		}
	}
	for _, di := range extra {
		add(di)
	}
	return out
}
