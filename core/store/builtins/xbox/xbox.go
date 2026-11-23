// Package xbox implementa a integração com o ecossistema Xbox / Microsoft
// Store. A autenticação usa o OAuth 2.0 de contas pessoais da Microsoft, troca
// o token por um token do Xbox Live e depois por um XSTS, e consulta o
// titlehub para montar o histórico de títulos da conta.
package xbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/playnux/core/models"
	"github.com/playnux/core/store"
)

const (
	defaultRedirectURI = "http://127.0.0.1:1"
	oauthScope         = "XboxLive.signin offline_access"
	loginUnavailable   = "Xbox sign-in is unavailable. Playnux needs its own Microsoft app registration with Xbox access."

	defaultAuthorizeBase = "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize"
	defaultTokenBase     = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"
	defaultXBLURL        = "https://user.auth.xboxlive.com/user/authenticate"
	defaultXSTSURL       = "https://xsts.auth.xboxlive.com/xsts/authorize"
	defaultTitlehubBase  = "https://titlehub.xboxlive.com"
	xboxRelyingParty     = "http://xboxlive.com"
)

type XboxProvider struct {
	auth          store.StoreAuth
	client        *http.Client
	clientID      string
	redirectURI   string
	authorizeBase string
	tokenBase     string
	xblURL        string
	xstsURL       string
	titlehubBase  string

	xuid      string
	userHash  string
	xstsToken string
}

func NewXboxProvider() *XboxProvider {
	return &XboxProvider{
		client:        &http.Client{Timeout: 30 * time.Second},
		clientID:      envOr("PLAYNUX_XBOX_CLIENT_ID", ""),
		redirectURI:   envOr("PLAYNUX_XBOX_REDIRECT_URI", defaultRedirectURI),
		authorizeBase: defaultAuthorizeBase,
		tokenBase:     defaultTokenBase,
		xblURL:        defaultXBLURL,
		xstsURL:       defaultXSTSURL,
		titlehubBase:  defaultTitlehubBase,
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (x *XboxProvider) ID() string { return "xbox" }

func (x *XboxProvider) Name() string { return "Xbox / Microsoft Store" }

// WebLogin abre o login oficial de contas pessoais da Microsoft. O redirect é
// o loopback registrado no client público; a ponte da UI captura a URL antes
// de a navegação ser concluída.
func (x *XboxProvider) WebLogin() store.WebLoginSpec {
	if x.clientID == "" {
		return store.WebLoginSpec{UnavailableReason: loginUnavailable}
	}
	query := url.Values{
		"client_id":     {x.clientID},
		"response_type": {"code"},
		"redirect_uri":  {x.redirectURI},
		"scope":         {oauthScope},
		"prompt":        {"select_account"},
	}
	return store.WebLoginSpec{
		URL:               x.authorizeBase + "?" + query.Encode(),
		RedirectURLPrefix: x.redirectURI,
	}
}

func (x *XboxProvider) Authenticate(ctx context.Context, auth store.StoreAuth) error {
	if x.clientID == "" {
		return fmt.Errorf("%s", loginUnavailable)
	}
	if auth.AccessToken == "" && auth.RefreshToken == "" {
		return fmt.Errorf("xbox: access token ou refresh token obrigatório")
	}
	x.auth = auth
	x.resetChain()
	if x.auth.AccessToken == "" || tokenExpired(x.auth.ExpiresAt) {
		if x.auth.RefreshToken == "" {
			return fmt.Errorf("xbox: token expirado e sem refresh token")
		}
		return x.refresh(ctx)
	}
	return nil
}

func (x *XboxProvider) IsAuthenticated() bool {
	return x.auth.AccessToken != "" || x.auth.RefreshToken != ""
}

func (x *XboxProvider) AuthSnapshot() store.StoreAuth { return x.auth }

func (x *XboxProvider) resetChain() {
	x.xuid = ""
	x.userHash = ""
	x.xstsToken = ""
}

// ExchangeCode troca o authorization code devolvido pelo login Microsoft.
func (x *XboxProvider) ExchangeCode(ctx context.Context, code string) (store.StoreAuth, error) {
	return x.requestToken(ctx, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {x.redirectURI},
		"scope":        {oauthScope},
	})
}

func (x *XboxProvider) refresh(ctx context.Context) error {
	auth, err := x.requestToken(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {x.auth.RefreshToken},
		"scope":         {oauthScope},
	})
	if err != nil {
		return err
	}
	if auth.RefreshToken == "" {
		auth.RefreshToken = x.auth.RefreshToken
	}
	auth.Cookies = x.auth.Cookies
	x.auth = auth
	x.resetChain()
	return nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (x *XboxProvider) requestToken(ctx context.Context, form url.Values) (store.StoreAuth, error) {
	if x.clientID == "" {
		return store.StoreAuth{}, fmt.Errorf("%s", loginUnavailable)
	}
	form.Set("client_id", x.clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.tokenBase,
		strings.NewReader(form.Encode()))
	if err != nil {
		return store.StoreAuth{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := x.client.Do(req)
	if err != nil {
		return store.StoreAuth{}, fmt.Errorf("xbox oauth: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return store.StoreAuth{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return store.StoreAuth{}, fmt.Errorf("xbox oauth retornou status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token tokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return store.StoreAuth{}, err
	}
	if token.AccessToken == "" {
		return store.StoreAuth{}, fmt.Errorf("xbox oauth: resposta sem access_token")
	}

	var expiresAt time.Time
	if token.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	return store.StoreAuth{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    expiresAt,
	}, nil
}

func tokenExpired(expiresAt time.Time) bool {
	return !expiresAt.IsZero() && time.Now().After(expiresAt.Add(-time.Minute))
}

func (x *XboxProvider) ensureToken(ctx context.Context) error {
	if x.auth.AccessToken != "" && !tokenExpired(x.auth.ExpiresAt) {
		return nil
	}
	if x.auth.RefreshToken == "" {
		return fmt.Errorf("xbox: sessão expirada, faça login novamente")
	}
	return x.refresh(ctx)
}

type xboxClaims struct {
	Token         string `json:"Token"`
	DisplayClaims struct {
		Xui []struct {
			UserHash string `json:"uhs"`
			Xid      string `json:"xid"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

// authenticateXBL troca o token OAuth pelo token de usuário do Xbox Live.
func (x *XboxProvider) authenticateXBL(ctx context.Context) (*xboxClaims, error) {
	payload := map[string]any{
		"Properties": map[string]string{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + x.auth.AccessToken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}
	var result xboxClaims
	if err := x.postXbox(ctx, x.xblURL, payload, &result); err != nil {
		return nil, fmt.Errorf("xbox user auth: %w", err)
	}
	if result.Token == "" {
		return nil, fmt.Errorf("xbox user auth: resposta sem token")
	}
	return &result, nil
}

// authorizeXSTS autoriza o token do Xbox Live para o relying party genérico
// (titlehub). O retorno contém XUID, user hash e o token final.
func (x *XboxProvider) authorizeXSTS(ctx context.Context, userToken string) (*xboxClaims, error) {
	payload := map[string]any{
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{userToken},
		},
		"RelyingParty": xboxRelyingParty,
		"TokenType":    "JWT",
	}
	var result xboxClaims
	if err := x.postXbox(ctx, x.xstsURL, payload, &result); err != nil {
		return nil, fmt.Errorf("xbox xsts: %w", err)
	}
	if result.Token == "" {
		return nil, fmt.Errorf("xbox xsts: resposta sem token")
	}
	if len(result.DisplayClaims.Xui) == 0 || result.DisplayClaims.Xui[0].Xid == "" {
		return nil, fmt.Errorf("xbox xsts: resposta sem XUID")
	}
	return &result, nil
}

func (x *XboxProvider) postXbox(ctx context.Context, endpoint string, payload any, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-xbl-contract-version", "1")

	resp, err := x.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return xboxStatusError(resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

func xboxStatusError(status int, body []byte) error {
	var payload struct {
		XErr int64 `json:"XErr"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.XErr != 0 {
		switch payload.XErr {
		case 2148916233:
			return fmt.Errorf("conta Microsoft não possui perfil Xbox; crie um em xbox.com")
		case 2148916235:
			return fmt.Errorf("Xbox Live não está disponível no país desta conta")
		case 2148916236, 2148916237:
			return fmt.Errorf("verificação de adulto necessária para esta conta Xbox")
		case 2148916238:
			return fmt.Errorf("conta infantil precisa ser adicionada a uma família")
		}
		return fmt.Errorf("Xbox retornou status %d (XErr %d): %s",
			status, payload.XErr, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("Xbox retornou status %d: %s", status, strings.TrimSpace(string(body)))
}

// ensureChain garante XUID e XSTS válidos para chamar os serviços do Xbox.
func (x *XboxProvider) ensureChain(ctx context.Context) error {
	if x.xstsToken != "" && x.userHash != "" && x.xuid != "" {
		return nil
	}
	if err := x.ensureToken(ctx); err != nil {
		return err
	}
	user, err := x.authenticateXBL(ctx)
	if err != nil {
		return err
	}
	xsts, err := x.authorizeXSTS(ctx, user.Token)
	if err != nil {
		return err
	}

	claims := xsts.DisplayClaims.Xui[0]
	x.xuid = claims.Xid
	x.userHash = claims.UserHash
	x.xstsToken = xsts.Token
	if x.auth.AccountID != x.xuid {
		x.auth.AccountID = x.xuid
	}
	return nil
}

type titleHistoryResponse struct {
	Titles []xboxTitle `json:"titles"`
	Paging struct {
		ContinuationToken string `json:"continuationToken"`
		TotalRecords      int    `json:"totalRecords"`
	} `json:"pagingInfo"`
}

type xboxTitle struct {
	TitleID string   `json:"titleId"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	PFN     string   `json:"pfn"`
	Devices []string `json:"devices"`
	Images  []struct {
		URL string `json:"url"`
	} `json:"images"`
	TitleHistory struct {
		LastTimePlayed string `json:"lastTimePlayed"`
	} `json:"titleHistory"`
	Detail struct {
		Availabilities []struct {
			ProductID string `json:"ProductId"`
		} `json:"availabilities"`
	} `json:"detail"`
}

// FetchLibrary consulta o histórico de títulos do Xbox Live. O Xbox não expõe
// uma biblioteca de compras públicas para consumidores; o titlehub é a fonte
// estável de títulos associados à conta (inclui Game Pass e jogos já jogados).
func (x *XboxProvider) FetchLibrary(ctx context.Context) ([]*models.Game, error) {
	if !x.IsAuthenticated() {
		return nil, fmt.Errorf("xbox: sessão não autenticada")
	}
	if err := x.ensureChain(ctx); err != nil {
		return nil, err
	}

	titles, err := x.fetchTitleHistory(ctx)
	if err != nil {
		return nil, err
	}

	games := make([]*models.Game, 0, len(titles))
	for _, title := range titles {
		if game := gameFromTitle(title); game != nil {
			games = append(games, game)
		}
	}
	sort.Slice(games, func(i, j int) bool {
		return strings.ToLower(games[i].Name) < strings.ToLower(games[j].Name)
	})
	return games, nil
}

func (x *XboxProvider) fetchTitleHistory(ctx context.Context) ([]xboxTitle, error) {
	var titles []xboxTitle
	token := ""
	seen := make(map[string]bool)
	attemptedRefresh := false

	for {
		endpoint := x.titlehubBase + "/users/xuid(" + url.PathEscape(x.xuid) +
			")/titles/titlehistory/decoration/detail,image,scid,achievement"
		query := url.Values{"maxItems": {strconv.Itoa(500)}}
		if token != "" {
			query.Set("continuationToken", token)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "XBL3.0 x="+x.userHash+";"+x.xstsToken)
		req.Header.Set("x-xbl-contract-version", "2")
		req.Header.Set("x-xbl-client-name", "XboxApp")
		req.Header.Set("x-xbl-client-type", "UWA")
		req.Header.Set("x-xbl-client-version", "39.39.22001.0")
		req.Header.Set("Accept-Language", "pt-BR")

		resp, err := x.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("xbox titlehub: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		if resp.StatusCode == http.StatusUnauthorized && !attemptedRefresh {
			attemptedRefresh = true
			x.resetChain()
			if err := x.ensureChain(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("xbox titlehub retornou status %d: %s",
				resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var page titleHistoryResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		titles = append(titles, page.Titles...)

		next := page.Paging.ContinuationToken
		if next == "" || seen[next] {
			break
		}
		seen[next] = true
		token = next
	}
	return titles, nil
}

func gameFromTitle(title xboxTitle) *models.Game {
	gameID := strings.TrimSpace(title.TitleID)
	if gameID == "" {
		gameID = strings.TrimSpace(title.PFN)
	}
	if gameID == "" || (title.Type != "" && !strings.EqualFold(title.Type, "game")) {
		return nil
	}

	name := strings.TrimSpace(title.Name)
	if name == "" {
		name = gameID
	}

	game := &models.Game{
		GameID:      gameID,
		PluginID:    "xbox",
		Source:      "Xbox",
		Name:        name,
		SortingName: name,
		Platforms:   platformsFor(title.Devices),
	}
	if len(title.Images) > 0 {
		image := normalizeURL(title.Images[0].URL)
		game.CoverImage = image
		game.BackgroundImage = image
	}
	if played := strings.TrimSpace(title.TitleHistory.LastTimePlayed); played != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, played); err == nil {
			game.LastActivity = &parsed
		}
	}
	return game
}

func platformsFor(devices []string) []string {
	seen := make(map[string]bool)
	var platforms []string
	add := func(platform string) {
		if platform == "" || seen[platform] {
			return
		}
		seen[platform] = true
		platforms = append(platforms, platform)
	}

	for _, device := range devices {
		switch strings.ToLower(device) {
		case "windows.desktop", "windows":
			add("PC (Windows)")
		case "xboxone":
			add("Xbox One")
		case "xboxseries", "xboxseriesx", "xboxseriess":
			add("Xbox Series X|S")
		case "xbox360":
			add("Xbox 360")
		}
	}
	if len(platforms) == 0 {
		platforms = []string{"Xbox"}
	}
	return platforms
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	return raw
}

func (x *XboxProvider) LaunchCommand(game *models.Game) (string, []string, error) {
	return "", nil, fmt.Errorf("xbox: o Playnux não executa jogos Xbox nativamente no Linux")
}

func (x *XboxProvider) UninstallCommand(game *models.Game) (string, []string, error) {
	return "", nil, fmt.Errorf("xbox: desinstalação não se aplica a jogos da nuvem Xbox")
}
