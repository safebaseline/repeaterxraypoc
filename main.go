package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type pocMeta struct {
	Name string `yaml:"name" json:"name"`
	Path string `yaml:"path" json:"path"`
}

type pocEntry struct {
	pocMeta
	FilePath string
	ModTime  time.Time
}

var usageText = `
Usage:
  go run . -dir <path-to-pocs> [-delete] [-out <output-dir>]

Examples:
  # Scan and show duplicate groups only
  go run . -dir ./pocs

  # Delete older duplicates while keeping the latest
  go run . -dir ./pocs -delete

  # Export deduplicated PoCs to another directory
  go run . -dir ./pocs -out ./deduped

  # Delete and export in one shot
  go run . -dir ./pocs -delete -out ./deduped
`

func main() {
	dirFlag := flag.String("dir", ".", "Directory containing xray PoCs")
	deleteFlag := flag.Bool("delete", false, "Delete duplicates keeping the most recently modified PoC")
	outFlag := flag.String("out", "", "Directory to write deduplicated PoCs")

	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), strings.TrimSpace(usageText))
		fmt.Fprintln(flag.CommandLine.Output(), "\nFlags:")
		flag.PrintDefaults()
	}

	flag.Parse()

	entries, err := collectPoCs(*dirFlag)
	if err != nil {
		log.Fatalf("collecting PoCs: %v", err)
	}
	if len(entries) == 0 {
		fmt.Println("No PoC files found.")
		return
	}

	groups := groupEntries(entries)
	duplicates := findDuplicates(groups)
	if len(duplicates) == 0 {
		fmt.Println("No duplicate PoCs detected based on path.")
		if *outFlag != "" {
			if err := exportDeduplicated(groups, *dirFlag, *outFlag); err != nil {
				log.Fatalf("exporting deduplicated PoCs: %v", err)
			}
			fmt.Printf("Deduplicated PoCs copied to %s\n", *outFlag)
		}
		return
	}

	printDuplicateReport(duplicates)

	if *deleteFlag {
		if err := deleteDuplicateFiles(duplicates); err != nil {
			log.Fatalf("deleting duplicates: %v", err)
		}
		fmt.Println("Duplicate files deleted (kept the most recent version for each path).")
	} else {
		fmt.Println("\nRun again with -delete to remove the older duplicates automatically.")
	}

	if *outFlag != "" {
		if err := exportDeduplicated(groups, *dirFlag, *outFlag); err != nil {
			log.Fatalf("exporting deduplicated PoCs: %v", err)
		}
		fmt.Printf("Deduplicated PoCs copied to %s\n", *outFlag)
	}
}

func collectPoCs(root string) ([]pocEntry, error) {
	var entries []pocEntry
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isSupportedExt(path) {
			return nil
		}
		fileEntries, err := loadPoC(path)
		if err != nil {
			log.Printf("Skipping %s: %v", path, err)
			return nil
		}
		entries = append(entries, fileEntries...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func isSupportedExt(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yml", ".yaml", ".json":
		return true
	default:
		return false
	}
}

func loadPoC(path string) ([]pocEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	paths := extractPathValues(&root)
	if len(paths) == 0 {
		return nil, errors.New("missing path field")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(findFirstScalar(&root, "name"))
	if name == "" {
		name = filepath.Base(path)
	}
	var entries []pocEntry
	for _, p := range paths {
		entries = append(entries, pocEntry{
			pocMeta: pocMeta{
				Name: name,
				Path: p,
			},
			FilePath: path,
			ModTime:  info.ModTime(),
		})
	}
	return entries, nil
}

func extractPathValues(node *yaml.Node) []string {
	seen := make(map[string]struct{})
	var out []string
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		switch n.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, child := range n.Content {
				walk(child)
			}
		case yaml.MappingNode:
			for i := 0; i < len(n.Content)-1; i += 2 {
				keyNode := n.Content[i]
				valNode := n.Content[i+1]
				if strings.EqualFold(strings.TrimSpace(keyNode.Value), "path") && valNode.Kind == yaml.ScalarNode {
					value := strings.TrimSpace(valNode.Value)
					if value != "" {
						if _, ok := seen[value]; !ok {
							seen[value] = struct{}{}
							out = append(out, value)
						}
					}
				}
				walk(valNode)
			}
		default:
			for _, child := range n.Content {
				walk(child)
			}
		}
	}
	walk(node)
	return out
}

func findFirstScalar(node *yaml.Node, key string) string {
	var result string
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil || result != "" {
			return
		}
		switch n.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, child := range n.Content {
				walk(child)
			}
		case yaml.MappingNode:
			for i := 0; i < len(n.Content)-1 && result == ""; i += 2 {
				keyNode := n.Content[i]
				valNode := n.Content[i+1]
				if strings.EqualFold(strings.TrimSpace(keyNode.Value), key) && valNode.Kind == yaml.ScalarNode {
					result = strings.TrimSpace(valNode.Value)
					return
				}
				walk(valNode)
			}
		}
	}
	if len(node.Content) > 0 {
		walk(node.Content[0])
	} else {
		walk(node)
	}
	return result
}

type duplicateGroup struct {
	Path    string
	Entries []pocEntry
}

func groupEntries(entries []pocEntry) map[string][]pocEntry {
	groupMap := map[string][]pocEntry{}
	for _, entry := range entries {
		key := entry.Path
		groupMap[key] = append(groupMap[key], entry)
	}
	for key, list := range groupMap {
		sort.Slice(list, func(i, j int) bool {
			return list[i].ModTime.After(list[j].ModTime)
		})
		groupMap[key] = list
	}
	return groupMap
}

func findDuplicates(groupMap map[string][]pocEntry) []duplicateGroup {
	var groups []duplicateGroup
	for path, list := range groupMap {
		if len(list) > 1 {
			groups = append(groups, duplicateGroup{
				Path:    path,
				Entries: list,
			})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Path < groups[j].Path
	})
	return groups
}

func printDuplicateReport(groups []duplicateGroup) {
	fmt.Printf("Detected %d duplicated path groups:\n", len(groups))
	for _, group := range groups {
		fmt.Printf("\nPath: %s\n", group.Path)
		for _, entry := range group.Entries {
			fmt.Printf("  - name=%q file=%s modified=%s\n", entry.Name, entry.FilePath, entry.ModTime.Format(time.RFC3339))
		}
		fmt.Printf("  * keep: %s\n", group.Entries[0].FilePath)
	}
}

func deleteDuplicateFiles(groups []duplicateGroup) error {
	deleted := make(map[string]struct{})
	for _, group := range groups {
		filesToDelete := group.Entries[1:]
		for _, entry := range filesToDelete {
			if _, ok := deleted[entry.FilePath]; ok {
				continue
			}
			if err := os.Remove(entry.FilePath); err != nil {
				return fmt.Errorf("remove %s: %w", entry.FilePath, err)
			}
			deleted[entry.FilePath] = struct{}{}
		}
	}
	return nil
}

func exportDeduplicated(groupMap map[string][]pocEntry, rootDir, outDir string) error {
	if outDir == "" {
		return nil
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return err
	}
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return err
	}

	paths := make([]string, 0, len(groupMap))
	for path := range groupMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		entries := groupMap[path]
		if len(entries) == 0 {
			continue
		}
		src := entries[0].FilePath
		absSrc, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(absRoot, absSrc)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = filepath.Base(absSrc)
		}
		dest := filepath.Join(absOut, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := copyFile(absSrc, dest); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	if src == dst {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
