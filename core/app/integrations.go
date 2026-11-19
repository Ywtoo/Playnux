package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/playnux/core/database"
	"github.com/playnux/core/metadata"
	"github.com/playnux/core/metadata/factory"
	"github.com/playnux/core/models"
	"github.com/playnux/core/store/builtins/steam"
)

const metadataPolicyKey = "metadata.policy"

type MetadataPolicy struct {
	Automatic bool     `json:"automatic"`
	Sources   []string `json:"sources"`
	Mode      string   `json:"mode"`
}

type MetadataSettings struct {
	Policy      MetadataPolicy  `json:"policy"`
	IGDB        bool            `json:"igdb"`
	SteamGridDB bool            `json:"steamgriddb"`
	SavedKeys   map[string]bool `json:"saved_keys"`
}

func (c *App) MetadataPolicy(ctx context.Context) (MetadataPolicy, error) {
	policy := MetadataPolicy{Sources: metadata.DefaultSources(), Mode: metadata.ModeMissing}
	raw, err := c.db.GetSetting(ctx, metadataPolicyKey)
	if errors.Is(err, database.ErrSettingNotFound) {
		return policy, nil
	}
	if err != nil {
		return policy, err
	}
	// Legacy settings only populate Automatic. Their individual modes are
	// replaced by the conservative default when no global rule was saved.
	err = json.Unmarshal([]byte(raw), &policy)
	return policy, err
}

func (c *App) MetadataSettings(ctx context.Context) (MetadataSettings, error) {
	policy, err := c.MetadataPolicy(ctx)
	if err != nil {
		return MetadataSettings{}, err
	}
	c.metadataMu.Lock()
	defer c.metadataMu.Unlock()
	saved := make(map[string]bool, 2)
	for _, id := range []string{"igdb", "steamgriddb"} {
		session, err := c.providerCredentials(ctx, id)
		if err != nil {
			return MetadataSettings{}, err
		}
		saved[id] = session.APIKey != ""
	}
	return MetadataSettings{
		Policy: policy, SavedKeys: saved,
		IGDB: c.metadata.Info != nil, SteamGridDB: c.metadata.Art != nil,
	}, nil
}

func validateMetadataPolicy(policy MetadataPolicy) error {
	if policy.Mode != metadata.ModeMissing && policy.Mode != metadata.ModeReplace {
		return fmt.Errorf("selecione uma regra de atualização válida")
	}
	if len(policy.Sources) == 0 {
		return fmt.Errorf("selecione ao menos uma fonte")
	}
	allowed := make(map[string]bool)
	for _, source := range metadata.DefaultSources() {
		allowed[source] = true
	}
	for _, source := range policy.Sources {
		if !allowed[source] {
			return fmt.Errorf("fonte de metadados desconhecida ou duplicada")
		}
		delete(allowed, source)
	}
	return nil
}

func (c *App) SaveMetadataPolicy(ctx context.Context, policy MetadataPolicy) error {
	if err := validateMetadataPolicy(policy); err != nil {
		return err
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return c.db.SetSetting(ctx, metadataPolicyKey, string(raw))
}

func (c *App) loadMetadataProviders(ctx context.Context) error {
	igdb, err := c.providerCredentials(ctx, "igdb")
	if err != nil {
		return err
	}
	grid, err := c.providerCredentials(ctx, "steamgriddb")
	if err != nil {
		return err
	}
	c.metadata = factory.New(igdb.AccountID, igdb.APIKey, grid.APIKey)
	return nil
}

func (c *App) providerCredentials(ctx context.Context, id string) (*models.Session, error) {
	session, err := c.db.LoadSession(ctx, "metadata:"+id)
	if errors.Is(err, database.ErrSessionNotFound) {
		return &models.Session{}, nil
	}
	return session, err
}

func (c *App) SetMetadataCredentials(ctx context.Context, id, clientID, key string) error {
	c.metadataMu.Lock()
	defer c.metadataMu.Unlock()
	clientID, key = strings.TrimSpace(clientID), strings.TrimSpace(key)
	if id != "igdb" && id != "steamgriddb" {
		return fmt.Errorf("provedor de metadados desconhecido")
	}
	if key == "" && clientID == "" {
		if err := c.db.DeleteSession(ctx, "metadata:"+id); err != nil {
			return err
		}
	} else {
		if key == "" || (id == "igdb" && clientID == "") {
			return fmt.Errorf("preencha as credenciais do provedor")
		}
		session := &models.Session{PluginID: "metadata:" + id, AccountID: clientID, APIKey: key}
		if err := c.db.SaveSession(ctx, session); err != nil {
			return err
		}
	}
	return c.loadMetadataProviders(ctx)
}

func (c *App) ConfigureSteam(ctx context.Context, accountID, key string, clearKey bool) error {
	accountID, key = strings.TrimSpace(accountID), strings.TrimSpace(key)
	if err := steam.ValidateAccountID(accountID); err != nil {
		return err
	}
	session, err := c.db.LoadSession(ctx, "steam")
	if errors.Is(err, database.ErrSessionNotFound) {
		session = &models.Session{PluginID: "steam"}
	} else if err != nil {
		return err
	}
	session.AccountID = accountID
	if clearKey {
		session.APIKey = ""
	} else if key != "" {
		session.APIKey = key
	}
	return c.db.SaveSession(ctx, session)
}
