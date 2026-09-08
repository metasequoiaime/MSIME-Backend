package account

import (
	"encoding/json"
	"os"
	"testing"
	"unicode/utf8"
)

func TestStarterSkinsUseAcceptedCommunityDesigns(t *testing.T) {
	raw, err := os.ReadFile("../../assets/community-starter-skins.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Publisher struct {
			ID   string `json:"id"`
			Name string `json:"display_name"`
		} `json:"publisher"`
		Skins []struct {
			ID          string          `json:"id"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Design      json.RawMessage `json:"design"`
		} `json:"skins"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if !validCommunityID(catalog.Publisher.ID) || catalog.Publisher.Name == "" || len(catalog.Skins) != 8 {
		t.Fatal("invalid starter catalog")
	}
	seen := map[string]bool{}
	for _, skin := range catalog.Skins {
		if seen[skin.ID] || len(skin.ID) != 36 || !validCommunityID(skin.ID) || utf8.RuneCountInString(skin.Name) > 32 || utf8.RuneCountInString(skin.Description) > 280 {
			t.Fatalf("invalid metadata: %s", skin.Name)
		}
		seen[skin.ID] = true
		if _, err := parseCommunityDesign(skin.Design); err != nil {
			t.Fatalf("%s: %v", skin.Name, err)
		}
	}
}
