# Requirements: Redmine Semantic Search (RSS)

| Feld | Wert |
|------|------|
| Projekt | Redmine Semantic Search (RSS) |
| Technologie | Qdrant, Go, Embedding API, REST |
| Version | 1.0 Draft |
| Datum | Februar 2026 |
| Autor | Olivier Dobberkau / dkd Internet Service GmbH |
| Status | Entwurf |

---

## 1. Projektziel

Redmine bietet von Haus aus eine rein textbasierte Suche, die bei wachsendem Datenbestand und heterogenen Inhalten (Issues, Wikis, Dokumente, Journale) schnell an ihre Grenzen stößt. Ziel dieses Projekts ist die Entwicklung einer semantischen Suchinfrastruktur auf Basis von Qdrant als Vektordatenbank. Nutzer sollen Inhalte über Bedeutung statt nur über exakte Zeichenketten finden können, wobei das bestehende Berechtigungsmodell von Redmine vollständig respektiert wird.

Die Lösung besteht aus zwei Kernkomponenten: einem in Go geschriebenen Indexer-Dienst, der Redmine-Inhalte kontinuierlich vektorisiert und in Qdrant ablegt, sowie einer Such-API, die Anfragen entgegennimmt, vektorisiert und gegen den Index abgleicht.

---

## 2. Stakeholder und Zielgruppen

**Endnutzer** sind alle Redmine-Anwender, die projektübergreifend oder innerhalb eines Projekts nach Informationen suchen. Sie profitieren von besserer Auffindbarkeit, auch wenn sie die exakte Formulierung eines Tickets nicht kennen.

**Administratoren** konfigurieren den Indexer, legen fest welche Projekte und Content-Typen indexiert werden, und überwachen den Betriebszustand.

**Entwickler** integrieren die Such-API in bestehende Oberflächen oder bauen eigene Frontends darauf auf.

---

## 3. Systemarchitektur (Übersicht)

Das System gliedert sich in drei Schichten:

**Indexierungsschicht (Go Indexer):** Ein eigenständiger Go-Service liest Inhalte über die Redmine REST API, extrahiert Text aus Anhängen, erzeugt Embeddings über ein konfigurierbares Embedding-Modell und schreibt die resultierenden Vektoren samt Metadaten als Payload in Qdrant.

**Speicherschicht (Qdrant):** Qdrant speichert Vektoren und strukturierte Metadaten in einer oder mehreren Collections. Payload-Indizes ermöglichen performante gefilterte Suchen nach Projekt, Status, Tracker, Autor und Content-Typ.

**Suchschicht (Search API):** Eine REST API (ebenfalls in Go oder als separater Service) nimmt Suchanfragen entgegen, vektorisiert sie, führt eine Nearest-Neighbor-Suche gegen Qdrant durch, filtert die Ergebnisse gegen Redmine-Berechtigungen und gibt sie sortiert nach Relevanz zurück.

---

## 4. Funktionale Anforderungen

### 4.1 Indexierung

| ID | Anforderung | Beschreibung | Priorität |
|----|-------------|--------------|-----------|
| FR-01 | Issue-Indexierung | Alle Issues (Titel, Beschreibung, Journal-Einträge, Custom Fields) werden als Vektoren in Qdrant indexiert. Unterstützung für inkrementelle Updates via Redmine REST API. | Muss |
| FR-02 | Wiki-Indexierung | Wiki-Seiten aller Projekte werden indexiert, einschließlich Textile/Markdown-Rendering zu Plaintext vor der Vektorisierung. | Muss |
| FR-03 | Dokument-Indexierung | Anhänge (PDF, DOCX, TXT, ODT) werden per Textextraktion verarbeitet und indexiert. Integration von Apache Tika oder einem vergleichbaren Extraktor als Sidecar-Service. | Soll |
| FR-04 | Journal-Indexierung | Kommentare und Statusänderungen (Journals) werden als eigenständige Vektoren indexiert und dem übergeordneten Issue zugeordnet. | Soll |
| FR-05 | Inkrementelle Updates | Der Indexer erkennt geänderte und neue Objekte seit dem letzten Lauf über das `updated_on`-Feld der Redmine API und indexiert nur diese nach. | Muss |
| FR-06 | Full Reindex | Ein vollständiger Neuaufbau des Index kann manuell oder per Konfiguration ausgelöst werden, ohne den laufenden Suchbetrieb zu unterbrechen (Blue-Green oder Alias-basiert). | Muss |
| FR-07 | Löschsynchronisation | Gelöschte Issues, Wiki-Seiten und Dokumente werden aus dem Qdrant-Index entfernt. Erkennung über Abgleich der vorhandenen IDs oder über Redmine-Webhooks. | Soll |

### 4.2 Suche

| ID | Anforderung | Beschreibung | Priorität |
|----|-------------|--------------|-----------|
| FR-10 | Semantische Suche | Suchanfragen werden vektorisiert und per Cosine Similarity gegen den Qdrant-Index abgeglichen. Ergebnisse werden nach Score sortiert zurückgegeben. | Muss |
| FR-11 | Hybrid Search | Kombination aus Vektorsuche (semantisch) und Sparse Vectors bzw. BM25 (Keyword-Match) mit konfigurierbarer Gewichtung. Besonders relevant für Ticket-Nummern, Fachbegriffe und Eigennamen. | Soll |
| FR-12 | Facettierte Filter | Einschränkung der Suchergebnisse nach Projekt, Tracker, Status, Autor, Zeitraum und Content-Typ über Qdrant Payload-Filter. | Muss |
| FR-13 | Paginierung | Suchergebnisse werden paginiert ausgeliefert mit konfigurierbarer Seitengröße (Default: 20). | Muss |
| FR-14 | Snippet-Generierung | Zu jedem Treffer wird ein Textausschnitt mit den relevantesten Passagen zurückgegeben. | Soll |
| FR-15 | Ähnliche Issues | Zu einem gegebenen Issue können semantisch ähnliche Issues abgefragt werden (More Like This). | Kann |
| FR-16 | Autocomplete / Suggest | Während der Eingabe werden Vorschläge auf Basis partieller Vektorsuche oder Präfix-Match angeboten. | Kann |

### 4.3 Berechtigungen und Sicherheit

| ID | Anforderung | Beschreibung | Priorität |
|----|-------------|--------------|-----------|
| FR-20 | Berechtigungsprüfung | Suchergebnisse werden gegen die Redmine-Berechtigungen des anfragenden Nutzers gefiltert. Nur Inhalte aus sichtbaren Projekten werden ausgegeben. | Muss |
| FR-21 | Pre-Filtering | Die erlaubten `project_ids` eines Nutzers werden als Qdrant-Filter übergeben, um nicht-autorisierte Dokumente gar nicht erst als Kandidaten zu laden. | Muss |
| FR-22 | API-Authentifizierung | Die Such-API authentifiziert Anfragen über Redmine API-Keys oder Session-Tokens und leitet die Identität an die Berechtigungsprüfung weiter. | Muss |
| FR-23 | Caching Berechtigungen | Projektmitgliedschaften und Rollen werden mit konfigurierbarer TTL gecacht, um die Last auf die Redmine-API zu reduzieren. | Soll |

### 4.4 API

| ID | Anforderung | Beschreibung | Priorität |
|----|-------------|--------------|-----------|
| FR-30 | REST API | Die Such-API stellt einen `GET /search`-Endpunkt bereit, der Query, Filter, Paginierung und Sortierung als Parameter akzeptiert. Antworten erfolgen in JSON. | Muss |
| FR-31 | Health Endpoint | Ein `GET /health`-Endpunkt liefert den Status des Dienstes, der Qdrant-Verbindung und des Embedding-Services. | Muss |
| FR-32 | Admin Endpoints | Geschützte Endpunkte zum Auslösen eines Full Reindex, Abfrage des Indexierungsstatus und Löschen einzelner Collections. | Soll |
| FR-33 | OpenAPI-Spezifikation | Die API wird über eine OpenAPI 3.x-Spezifikation dokumentiert. | Soll |

---

## 5. Nicht-funktionale Anforderungen

### 5.1 Performance

| ID | Anforderung | Zielwert |
|----|-------------|----------|
| NF-01 | Suchantwortzeit | < 200 ms (P95) für eine semantische Suche über bis zu 500.000 Vektoren |
| NF-02 | Indexierungsdurchsatz | ≥ 100 Dokumente pro Sekunde bei inkrementeller Indexierung |
| NF-03 | Full Reindex | Vollständige Neuindexierung von 100.000 Issues innerhalb von 30 Minuten |
| NF-04 | Embedding-Latenz | < 50 ms pro Einzelanfrage an das Embedding-Modell |

### 5.2 Skalierbarkeit und Betrieb

| ID | Anforderung | Beschreibung |
|----|-------------|--------------|
| NF-10 | Containerisierung | Alle Komponenten (Indexer, Search API, Qdrant) werden als Docker-Container bereitgestellt und über Docker Compose oder Kubernetes orchestriert. |
| NF-11 | Konfiguration | Sämtliche Parameter (Redmine-URL, API-Key, Qdrant-Endpunkt, Embedding-Modell, Batch-Größen, Intervalle) werden über Umgebungsvariablen oder eine YAML-Konfigurationsdatei gesteuert. |
| NF-12 | Logging | Strukturiertes Logging (JSON) mit konfigurierbarem Log-Level. Indexierungsfortschritt, Fehler und Metriken werden protokolliert. |
| NF-13 | Monitoring | Prometheus-kompatible Metriken für Indexierungsrate, Suchlatenz, Queue-Tiefe und Fehlerrate. |
| NF-14 | Graceful Shutdown | Sauberes Herunterfahren mit Abschluss laufender Batch-Operationen. |

### 5.3 Zuverlässigkeit

| ID | Anforderung | Beschreibung |
|----|-------------|--------------|
| NF-20 | Idempotenz | Wiederholte Indexierung desselben Dokuments führt zum gleichen Ergebnis (Upsert-Semantik). |
| NF-21 | Fehlertoleranz | Temporäre Ausfälle von Redmine, Qdrant oder dem Embedding-Service führen zu Retry mit exponentiellem Backoff, nicht zum Abbruch. |
| NF-22 | Datenkonsistenz | Nach einem Full Reindex enthält der Index exakt die Menge der aktuell in Redmine vorhandenen und für die Indexierung konfigurierten Inhalte. |

---

## 6. Datenmodell

### 6.1 Qdrant Collection

**Collection Name:** `redmine_search`

**Vektorkonfiguration:**

| Parameter | Wert |
|-----------|------|
| Dimension | Abhängig vom Embedding-Modell (z. B. 384 für MiniLM, 1536 für OpenAI) |
| Distanzmetrik | Cosine |
| On-Disk | true (für Instanzen > 100k Vektoren empfohlen) |

**Payload-Schema:**

| Feld | Typ | Beschreibung | Index |
|------|-----|--------------|-------|
| `redmine_id` | integer | ID des Objekts in Redmine | Ja |
| `content_type` | keyword | `issue`, `wiki`, `document`, `journal` | Ja |
| `project_id` | integer | Redmine Projekt-ID | Ja |
| `project_identifier` | keyword | Redmine Projekt-Slug | Nein |
| `tracker` | keyword | Tracker-Name (nur Issues) | Ja |
| `status` | keyword | Status-Name (nur Issues) | Ja |
| `priority` | keyword | Priorität (nur Issues) | Nein |
| `author` | keyword | Autor/Ersteller | Ja |
| `assigned_to` | keyword | Zugewiesener Bearbeiter | Nein |
| `subject` | text | Titel/Betreff | Nein |
| `text_preview` | text | Erste 500 Zeichen des Inhalts für Snippet-Anzeige | Nein |
| `created_on` | datetime | Erstellungsdatum | Ja |
| `updated_on` | datetime | Letzte Änderung | Ja |
| `parent_id` | integer | Übergeordnetes Objekt (z. B. Issue-ID bei Journals) | Nein |
| `url` | text | Direkt-Link zum Objekt in Redmine | Nein |

### 6.2 Sparse Vectors (Hybrid Search)

Für die Hybrid-Suche wird ein zweiter, benannter Vektor vom Typ Sparse konfiguriert. Die Sparse-Repräsentation wird über ein SPLADE-Modell oder einen BM25-kompatiblen Tokenizer erzeugt. Qdrant unterstützt benannte Vektoren, sodass Dense und Sparse Vektoren in derselben Collection koexistieren.

---

## 7. Technologie-Stack

### 7.1 Indexer (Go)

| Komponente | Technologie | Begründung |
|------------|-------------|------------|
| Sprache | Go 1.22+ | Hoher Durchsatz, einfaches Deployment als Single Binary, exzellente Concurrency für parallele API-Aufrufe und Batch-Verarbeitung |
| Qdrant Client | `github.com/qdrant/go-client` | Offizieller gRPC-basierter Go Client |
| HTTP Client | `net/http` (stdlib) | Für Redmine REST API und Embedding API |
| Textextraktion | Apache Tika (Sidecar-Container) | Bewährte Lösung für PDF, DOCX, ODT. Ansprache über Tika REST API |
| Konfiguration | `github.com/spf13/viper` | YAML + Environment Variables |
| Logging | `log/slog` (stdlib) | Strukturiertes Logging, ab Go 1.21 in der Standardbibliothek |
| Scheduling | `github.com/robfig/cron/v3` | Cron-basierte Steuerung der Indexierungsintervalle |

### 7.2 Embedding

| Option | Modell | Dimension | Bemerkung |
|--------|--------|-----------|-----------|
| Lokal (Self-Hosted) | `sentence-transformers/all-MiniLM-L6-v2` | 384 | Schnell, ressourcenschonend, gute Ergebnisse für Englisch |
| Lokal (Deutsch) | `deepset/gbert-base` oder `intfloat/multilingual-e5-base` | 768 | Bessere Performance für deutsche Inhalte |
| Cloud | OpenAI `text-embedding-3-small` | 1536 | Höchste Qualität, erfordert API-Key und erlaubt Datenübertragung an Dritte |

Das Embedding-Modell wird als austauschbare Komponente hinter einer einheitlichen Schnittstelle (`Embedder` Interface in Go) angebunden. Ein lokales Modell kann über einen leichtgewichtigen HTTP-Service (z. B. FastAPI + sentence-transformers oder TEI von Hugging Face) bereitgestellt werden.

### 7.3 Infrastruktur

| Komponente | Technologie |
|------------|-------------|
| Vektordatenbank | Qdrant (Docker, aktuelles Stable Release) |
| Textextraktion | Apache Tika 2.x (Docker) |
| Embedding Service | Hugging Face TEI oder eigener FastAPI-Service |
| Orchestrierung | Docker Compose (Entwicklung/kleine Instanzen), Kubernetes (Produktion) |
| Reverse Proxy | Nginx oder Traefik (optional, für TLS-Terminierung) |

---

## 8. Indexer-Architektur (Go)

### 8.1 Modulstruktur

```
redmine-search-indexer/
├── cmd/
│   └── indexer/
│       └── main.go              # Einstiegspunkt, CLI-Flags
├── internal/
│   ├── config/
│   │   └── config.go            # Konfiguration laden und validieren
│   ├── redmine/
│   │   ├── client.go            # Redmine REST API Client
│   │   ├── issues.go            # Issue-spezifische API-Aufrufe
│   │   ├── wikis.go             # Wiki-spezifische API-Aufrufe
│   │   └── models.go            # Redmine Datenstrukturen
│   ├── embedder/
│   │   ├── embedder.go          # Interface-Definition
│   │   ├── openai.go            # OpenAI Implementierung
│   │   └── local.go             # Lokaler HTTP-Service Implementierung
│   ├── extractor/
│   │   └── tika.go              # Apache Tika Client für Textextraktion
│   ├── indexer/
│   │   ├── indexer.go           # Orchestrierung der Pipeline
│   │   ├── batch.go             # Batch-Verarbeitung und Retry-Logik
│   │   └── sync.go              # Inkrementelle und Full-Sync-Logik
│   ├── qdrant/
│   │   ├── client.go            # Qdrant gRPC Client Wrapper
│   │   └── collection.go        # Collection-Management
│   └── metrics/
│       └── prometheus.go        # Metriken-Export
├── api/
│   ├── server.go                # HTTP Server für Search API
│   ├── handlers.go              # Request Handler
│   └── middleware.go            # Auth, Logging, CORS
├── deployments/
│   ├── docker-compose.yml
│   ├── Dockerfile
│   └── config.example.yml
└── go.mod
```

### 8.2 Indexierungs-Pipeline

Der Indexer arbeitet in folgenden Schritten:

Schritt 1 — Abruf: Der Redmine-Client ruft über die REST API alle Objekte ab, die seit dem letzten Sync-Zeitpunkt geändert wurden. Die Abfrage erfolgt paginiert mit konfigurierbarer Batch-Größe (Default: 100). Parallele Goroutinen verarbeiten unterschiedliche Content-Typen gleichzeitig.

Schritt 2 — Textaufbereitung: Für jedes Objekt wird der indexierbare Text zusammengesetzt. Bei Issues umfasst das Titel, Beschreibung und optionale Custom Fields. Textile- oder Markdown-Formatierung wird zu Plaintext konvertiert. Anhänge werden an Apache Tika zur Textextraktion übergeben.

Schritt 3 — Embedding: Die aufbereiteten Texte werden in Batches an den Embedding-Service übergeben. Die Batch-Größe ist konfigurierbar und sollte an das Rate-Limit des Embedding-Providers angepasst werden. Lange Texte werden vor der Vektorisierung auf die maximale Token-Länge des Modells gekürzt oder in Chunks aufgeteilt.

Schritt 4 — Upsert: Die resultierenden Vektoren werden zusammen mit den Metadaten (Payload) per Batch-Upsert in Qdrant geschrieben. Die Punkt-ID in Qdrant wird deterministisch aus Content-Typ und Redmine-ID abgeleitet, um Idempotenz zu gewährleisten.

Schritt 5 — Sync-State: Nach erfolgreichem Durchlauf wird der Zeitstempel des letzten Syncs persistiert (Datei oder Qdrant Payload auf einem Metadaten-Punkt).

### 8.3 Chunking-Strategie

Dokumente, die die maximale Token-Länge des Embedding-Modells überschreiten, werden in überlappende Chunks aufgeteilt. Jeder Chunk wird als eigenständiger Vektor gespeichert und über `parent_id` dem Quelldokument zugeordnet. Empfohlene Parameter: Chunk-Größe 512 Tokens, Überlappung 50 Tokens. Bei der Suche werden Ergebnisse desselben Quelldokuments dedupliziert und der höchste Score verwendet.

---

## 9. Such-API

### 9.1 Endpunkte

**`GET /api/v1/search`**

| Parameter | Typ | Pflicht | Beschreibung |
|-----------|-----|---------|--------------|
| `q` | string | Ja | Suchanfrage (Freitext) |
| `project_id` | integer | Nein | Einschränkung auf ein Projekt |
| `content_type` | string | Nein | Filter: `issue`, `wiki`, `document`, `journal` |
| `tracker` | string | Nein | Filter: Tracker-Name |
| `status` | string | Nein | Filter: Status-Name (oder `open`, `closed`) |
| `author` | string | Nein | Filter: Autorenname |
| `date_from` | date | Nein | Ergebnisse ab diesem Datum |
| `date_to` | date | Nein | Ergebnisse bis zu diesem Datum |
| `limit` | integer | Nein | Ergebnisse pro Seite (Default: 20, Max: 100) |
| `offset` | integer | Nein | Offset für Paginierung |
| `hybrid_weight` | float | Nein | Gewichtung Keyword vs. Semantik (0.0 = rein semantisch, 1.0 = rein Keyword, Default: 0.3) |

**Antwortformat:**

```json
{
  "query": "Login-Problem nach Update",
  "total": 42,
  "limit": 20,
  "offset": 0,
  "results": [
    {
      "redmine_id": 12345,
      "content_type": "issue",
      "project": "website-relaunch",
      "tracker": "Bug",
      "status": "In Bearbeitung",
      "subject": "SSO-Authentifizierung schlägt nach 4.2 Update fehl",
      "snippet": "Nach dem Update auf Version 4.2 können sich Nutzer nicht mehr über SSO anmelden...",
      "score": 0.89,
      "url": "https://redmine.example.com/issues/12345",
      "updated_on": "2026-02-10T14:30:00Z"
    }
  ]
}
```

**`GET /api/v1/similar/{content_type}/{id}`**

Gibt semantisch ähnliche Objekte zu einem gegebenen Redmine-Objekt zurück.

**`GET /api/v1/health`**

Liefert Statusinfomationen zu allen Abhängigkeiten (Qdrant, Embedding-Service, Redmine-Erreichbarkeit).

**`POST /api/v1/admin/reindex`** (geschützt)

Löst einen vollständigen Neuaufbau des Index aus.

---

## 10. Berechtigungskonzept

Die Berechtigungsprüfung erfolgt zweistufig:

**Pre-Filtering:** Beim Eingang einer Suchanfrage wird über die Redmine API (oder einen lokalen Cache) ermittelt, auf welche Projekte der authentifizierte Nutzer Zugriff hat. Diese `project_ids` werden als `must`-Filter an die Qdrant-Suche übergeben. Dadurch werden nicht-autorisierte Dokumente gar nicht erst als Kandidaten evaluiert, was sowohl die Sicherheit als auch die Performance verbessert.

**Post-Filtering (Fallback):** Falls feinere Berechtigungen auf Issue-Ebene greifen (z. B. private Issues), erfolgt nach der Qdrant-Abfrage eine zusätzliche Prüfung. In diesem Fall fordert die API mehr Ergebnisse von Qdrant an als letztlich ausgeliefert werden (Oversampling-Faktor konfigurierbar, Default: 2x).

**Caching:** Projektmitgliedschaften und Rollen werden mit einer TTL von 5 Minuten gecacht, um die Redmine-API nicht bei jeder Suchanfrage zu belasten. Der Cache wird bei expliziter Invalidierung oder nach TTL-Ablauf aktualisiert.

---

## 11. Deployment

### 11.1 Docker Compose (Entwicklung / kleine Instanzen)

```yaml
services:
  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - qdrant_data:/qdrant/storage

  embedding:
    image: ghcr.io/huggingface/text-embeddings-inference:latest
    command: --model-id sentence-transformers/all-MiniLM-L6-v2
    ports:
      - "8080:80"

  tika:
    image: apache/tika:latest
    ports:
      - "9998:9998"

  indexer:
    build: .
    depends_on: [qdrant, embedding, tika]
    environment:
      - REDMINE_URL=https://redmine.example.com
      - REDMINE_API_KEY=${REDMINE_API_KEY}
      - QDRANT_HOST=qdrant
      - QDRANT_PORT=6334
      - EMBEDDING_URL=http://embedding:80
      - TIKA_URL=http://tika:9998
      - INDEXER_INTERVAL=5m
    ports:
      - "8090:8090"

volumes:
  qdrant_data:
```

### 11.2 Mindestanforderungen

| Komponente | CPU | RAM | Disk |
|------------|-----|-----|------|
| Qdrant | 2 Cores | 4 GB | 10 GB (abhängig von Vektoranzahl) |
| Embedding Service | 2 Cores (4 mit GPU empfohlen) | 4 GB | 2 GB (Modell-Download) |
| Indexer + Search API | 1 Core | 512 MB | minimal |
| Apache Tika | 1 Core | 1 GB | minimal |

---

## 12. Konfiguration

```yaml
redmine:
  url: "https://redmine.example.com"
  api_key: "${REDMINE_API_KEY}"
  batch_size: 100
  projects: []           # Leer = alle Projekte
  content_types:
    - issue
    - wiki
    - journal
    - document

qdrant:
  host: "localhost"
  grpc_port: 6334
  collection: "redmine_search"
  batch_size: 100

embedding:
  provider: "local"      # "local" oder "openai"
  url: "http://localhost:8080"
  model: "sentence-transformers/all-MiniLM-L6-v2"
  batch_size: 32
  max_tokens: 512

indexer:
  interval: "5m"
  full_reindex_cron: "0 2 * * 0"   # Sonntags 02:00
  workers: 4
  chunking:
    enabled: true
    chunk_size: 512
    overlap: 50

search:
  listen: ":8090"
  default_limit: 20
  max_limit: 100
  oversampling_factor: 2
  hybrid_weight: 0.3

auth:
  cache_ttl: "5m"

tika:
  url: "http://localhost:9998"
  timeout: "30s"
```

---

## 13. Testanforderungen

**Unit Tests:** Alle Kernmodule (Redmine-Client, Embedder-Interface, Qdrant-Client, Berechtigungslogik) werden mit Unit Tests abgedeckt. Ziel ist eine Testabdeckung von mindestens 80% der Geschäftslogik.

**Integrationstests:** Ein Testsetup mit einer Redmine-Testinstanz, Qdrant und einem Embedding-Service prüft die End-to-End-Pipeline von der Indexierung bis zur Suche. Diese Tests laufen in einer CI/CD-Pipeline (GitHub Actions oder GitLab CI).

**Performance-Tests:** Lasttests mit realistischen Datenmengen validieren die in Abschnitt 5.1 definierten Zielwerte. Werkzeuge: `k6` oder `vegeta` für HTTP-Lasttests.

**Qualitätstests:** Ein Satz von Referenz-Suchanfragen mit erwarteten Ergebnissen wird als Regression Suite gepflegt, um die Suchqualität bei Modell- oder Konfigurationsänderungen zu überwachen.

---

## 14. Offene Punkte und Entscheidungen

**Embedding-Modell:** Die endgültige Modellwahl hängt davon ab, ob die Redmine-Inhalte primär deutsch, englisch oder mehrsprachig sind. Ein Benchmark mit realen Daten sollte vor der Produktivsetzung durchgeführt werden.

**Chunking vs. Truncation:** Für kurze Inhalte wie Issues reicht Truncation. Für umfangreiche Wiki-Seiten und Dokumente ist Chunking sinnvoller. Die Grenze sollte empirisch ermittelt werden.

**Redmine-Plugin vs. eigenständige UI:** Ein Redmine-Plugin, das die native Suchseite ersetzt, bietet die nahtloseste Integration, erfordert aber Ruby-Entwicklung. Eine separate Web-UI ist schneller realisierbar, erzeugt aber einen Medienbruch.

**Webhook vs. Polling:** Redmine bietet standardmäßig keine Webhooks. Entweder wird ein Webhook-Plugin installiert oder der Indexer pollt in konfigurierbaren Intervallen. Für den MVP wird Polling empfohlen.

**Datenschutz und Datensouveränität:** Bei Verwendung eines Cloud-Embedding-Services (OpenAI) verlassen Redmine-Inhalte das eigene Netzwerk. Für sensible Daten sollte ein lokales Modell eingesetzt werden.

---

## 15. Meilensteine

| Phase | Inhalt | Zeitraum |
|-------|--------|----------|
| M1 — Proof of Concept | Go Indexer für Issues, Qdrant-Setup, einfache Vektorsuche ohne Berechtigungen | 2 Wochen |
| M2 — Kernfunktionalität | Wiki- und Journal-Indexierung, Berechtigungsprüfung, REST API mit Filtern | 3 Wochen |
| M3 — Hybrid Search & Qualität | Sparse Vectors, Hybrid Search, Snippet-Generierung, Qualitäts-Benchmark | 2 Wochen |
| M4 — Dokumente & Extraktion | Attachment-Indexierung via Tika, Chunking-Strategie | 2 Wochen |
| M5 — Produktion | Monitoring, Logging, Docker-Compose-Setup, Dokumentation, Performance-Tests | 2 Wochen |
