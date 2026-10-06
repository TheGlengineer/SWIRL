package main

// Card labels (2.17): a name of the owner's choosing for a card ("Main card", "RPGs", "Kids"), kept on the
// card itself in SWIRL/card.json so it travels with the card between PCs. The drive list and the card chip
// show it, which is how an owner with several cards tells them apart before scanning.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const cardLabelMax = 24

type cardInfoFile struct {
	Label string `json:"label,omitempty"`
}

func cardInfoPath(root string) string { return filepath.Join(root, editsDir, "card.json") }

// CardLabel reads a card's label, "" when it has none.
func CardLabel(root string) string {
	b, err := os.ReadFile(cardInfoPath(root))
	if err != nil {
		return ""
	}
	var ci cardInfoFile
	if json.Unmarshal(b, &ci) != nil {
		return ""
	}
	return ci.Label
}

// SetCardLabel stores the label; an empty one removes it. Other keys in card.json are kept.
func SetCardLabel(root, label string) error {
	label = strings.TrimSpace(strings.Join(strings.Fields(label), " "))
	if len([]rune(label)) > cardLabelMax {
		return errors.New("a card label is at most 24 characters")
	}
	if _, err := os.Stat(filepath.Join(root, "01")); err != nil {
		if _, err2 := os.Stat(root); err2 != nil {
			return errors.New("that card is not in the reader")
		}
	}
	raw := map[string]any{}
	if b, err := os.ReadFile(cardInfoPath(root)); err == nil {
		json.Unmarshal(b, &raw)
	}
	if label == "" {
		delete(raw, "label")
	} else {
		raw["label"] = label
	}
	if len(raw) == 0 {
		os.Remove(cardInfoPath(root))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cardInfoPath(root)), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(raw, "", "  ")
	return writeCardFile(cardInfoPath(root), b)
}
