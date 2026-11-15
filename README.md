# Playnux

> **Your games. One library. Every platform.**

🐧 **Linux-first** · 🪟 **Windows support** · 🔓 **Open Source** · 🤝 **Community-driven**

O **Playnux** é um gerenciador de biblioteca de jogos independente, modular e de altíssimo desempenho, construído com foco primário em Linux e suporte de primeira classe a Windows. Inspirado nos conceitos do Playnite, mas redesenhado do zero para máxima velocidade, extensibilidade segura e liberdade estética comunitária.

---

## 🏗️ Stack Tecnológica

| Camada | Tecnologia | Papel |
| :--- | :--- | :--- |
| **Lógica / Core** | **Go** | Regras de negócio, busca em milissegundos, scrapers de lojas, concorrência e auditoria de segurança. |
| **Banco de Dados** | **SQLite (WAL)** | Persistência ultra-rápida em arquivo local, transacional e com zero overhead de rede. |
| **Ponte de Interface** | **Wails** | Conecta o Core Go à Webview nativa do SO (WebKitGTK no Linux, WebView2 no Windows). Sem Electron. |
| **Interface / Temas** | **HTML5 + CSS3 + TypeScript** | Liberdade total de layout para temas (PS5, Big Picture, Minimalista). |
| **Plugins** | **Processos Isolados + JSON-RPC 2.0** | Extensões em qualquer linguagem (Go, Python, Rust, C#, JS) sem risco de derrubar o app. |
| **Ciclo de Vida** | **Gameplay Deep Sleep** | Durante gameplay, a UI congela e o Core entra em standby (0% CPU, 0% GPU, zero perda de FPS). |

---

## 🎮 Fontes locais, metadados e emulação

Além das lojas digitais, o Playnux lê o próprio Linux:

* **Desktop:** entradas `.desktop` com `Categories=Game;` em `/usr/share/applications` e `~/.local/share/applications`.
* **Flatpak:** atalhos exportados em `~/.local/share/flatpak/exports/share/applications` e `/var/lib/flatpak/exports/share/applications`; o jogo é aberto com `flatpak run <app_id>`.
* **DRM-free:** qualquer pasta indicada pela UI; o scanner encontra ELF com permissão `+x` e scripts `.sh`.
* **Emulação:** detecta RetroArch, PCSX2, RPCS3, Dolphin, DuckStation, Ryujinx e PPSSPP (binário no `PATH` ou Flatpak) e varre `~/ROMs`, `~/Emulation/roms` e pastas extras.

O drawer de detalhes tem um botão **Metadados** que consulta o IGDB e o SteamGridDB sob demanda. Configure as chaves no ambiente antes de abrir o app:

```bash
export IGDB_CLIENT_ID="seu_client_id"
export IGDB_CLIENT_SECRET="seu_client_secret"
export STEAMGRIDDB_API_KEY="sua_chave"
```

`TWITCH_CLIENT_ID`/`TWITCH_CLIENT_SECRET` também são aceitos como alternativa para o IGDB. Sem as chaves, a biblioteca e o lançamento continuam funcionando; apenas o enriquecimento fica desativado. Trailers do YouTube são reproduzidos dentro do drawer.

---

## 📚 Documentação do Projeto

Toda a arquitetura e decisões de engenharia estão documentadas detalhadamente na pasta [`docs/`](docs/):

### Especificações Principais
* 📄 **[Plano Principal do Projeto](docs/playnux-plano.md):** Metas, fases de desenvolvimento e princípios de memória.
* 📄 **[Especificação Mestre (Playnux Master Spec)](docs/architecture/PLAYNUX_MASTER_SPEC.md):** Detalhamento da arquitetura completa, modelo de temas em 2 níveis, segurança e auditoria pelo Core, slots de plugins e Gameplay Deep Sleep.
* 📄 **[UI and Plugin Specification](docs/architecture/UI_AND_PLUGIN_SPEC.md):** Contrato agnóstico de IPC, regiões de extensão e perfis de execução.
* 📄 **[Estudo Técnico do Playnite](docs/playnite-estudo.md):** Análise do código original (C#/WPF), mapeamento de dependências Windows e o que foi aproveitado como referência.

### Registros de Decisão Arquitetural (ADRs)
* 📋 **[ADR-001: Core em Go com Banco SQLite](docs/decisions/ADR-001-core-go-sqlite.md)**
* 📋 **[ADR-002: Independência de Renderizador e Contrato de Temas](docs/decisions/ADR-002-renderer-independence.md)**
* 📋 **[ADR-003: Isolamento de Plugins e Modo Deep Sleep em Gameplay](docs/decisions/ADR-003-plugin-isolation-and-gameplay-sleep.md)**
* 📋 **[ADR-004: Stack de Interface com Wails e Webview Nativo](docs/decisions/ADR-004-wails-webview-ui.md)**

---

## 📁 Estrutura de Diretórios

```
Playnux/
├── docs/                      → Documentação oficial e ADRs
│   ├── architecture/          → Especificações técnicas detalhadas
│   └── decisions/             → Architecture Decision Records (ADRs)
├── proto-gio/                 → Protótipo experimental de UI com Gio
├── proto-slint/               → Protótipo experimental de UI com Slint
├── benchmark/                 → Scripts de teste de estresse de memória e CPU
├── _playnite-reference/       → Código original do Playnite (apenas referência, fora do build)
└── README.md
```

---

## ⚖️ Licença e Créditos

Distribuído sob a licença **MIT**.  
Inspirado pelos conceitos de gerenciamento de biblioteca do projeto [Playnite](https://github.com/JosefNemec/Playnite) de Josef Nemec (MIT).
