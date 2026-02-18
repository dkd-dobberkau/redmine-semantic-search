# Requirements: Redmine Semantic Search (RSS)

**Defined:** 2026-02-18
**Core Value:** Nutzer finden relevante Redmine-Inhalte über semantische Suche, auch wenn sie die exakte Formulierung nicht kennen — ohne das Berechtigungsmodell zu umgehen.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Indexierung

- [ ] **IDX-01**: Issue-Indexierung — Titel, Beschreibung und Custom Fields werden als Vektoren in Qdrant indexiert
- [ ] **IDX-02**: Wiki-Indexierung — Wiki-Seiten aller Projekte werden indexiert, Textile/Markdown wird zu Plaintext konvertiert
- [ ] **IDX-03**: Journal-Indexierung — Kommentare und Statusänderungen werden als eigenständige Vektoren indexiert und dem übergeordneten Issue zugeordnet
- [ ] **IDX-04**: Inkrementelle Updates — Geänderte und neue Objekte werden über `updated_on` erkannt und nachindexiert
- [ ] **IDX-05**: Full Reindex — Vollständiger Neuaufbau des Index ohne Suchunterbrechung (Collection-Alias-basiert)
- [ ] **IDX-06**: Löschsynchronisation — Gelöschte Issues, Wiki-Seiten werden aus dem Index entfernt (ID-Reconciliation)
- [ ] **IDX-07**: Textaufbereitung — Textile/Markdown-Formatierung wird zu Plaintext konvertiert, Texte werden auf maximale Token-Länge gekürzt oder in Chunks aufgeteilt

### Suche

- [ ] **SRCH-01**: Semantische Suche — Suchanfragen werden vektorisiert und per Cosine Similarity gegen Qdrant abgeglichen, sortiert nach Score
- [ ] **SRCH-02**: Hybrid Search — Kombination aus Vektorsuche und Sparse Vectors/BM25 mit konfigurierbarer Gewichtung (`hybrid_weight` Parameter)
- [ ] **SRCH-03**: Facettierte Filter — Einschränkung nach Projekt, Tracker, Status, Autor, Zeitraum und Content-Typ über Qdrant Payload-Filter
- [ ] **SRCH-04**: Paginierung — Ergebnisse werden paginiert ausgeliefert (Default: 20, Max: 100)
- [ ] **SRCH-05**: Snippet-Generierung — Zu jedem Treffer wird ein Textausschnitt mit den relevantesten Passagen zurückgegeben
- [ ] **SRCH-06**: Ähnliche Issues — Zu einem gegebenen Issue können semantisch ähnliche Issues abgefragt werden (More Like This)

### Berechtigungen

- [ ] **AUTH-01**: Permission Pre-Filtering — Erlaubte `project_ids` des Nutzers werden als Qdrant-Filter übergeben
- [ ] **AUTH-02**: API-Authentifizierung — Anfragen werden über Redmine API-Keys oder Session-Tokens authentifiziert
- [ ] **AUTH-03**: Post-Filtering — Feinere Berechtigungen (private Issues) werden nach der Qdrant-Abfrage geprüft (Oversampling)

### API

- [ ] **API-01**: REST Search Endpoint — `GET /api/v1/search` mit Query, Filter, Paginierung und Sortierung als Parameter, JSON-Antworten
- [ ] **API-02**: Health Endpoint — `GET /api/v1/health` liefert Status des Dienstes, Qdrant-Verbindung und Embedding-Service
- [ ] **API-03**: Similar Endpoint — `GET /api/v1/similar/{content_type}/{id}` gibt semantisch ähnliche Objekte zurück
- [ ] **API-04**: Admin Reindex Endpoint — `POST /api/v1/admin/reindex` löst Full Reindex aus (geschützt)
- [ ] **API-05**: OpenAPI-Spezifikation — API wird über OpenAPI 3.x dokumentiert

### Betrieb

- [ ] **OPS-01**: Docker-Compose-Deployment — Alle Komponenten (Indexer, Search API, Qdrant, Embedding-Service) als Docker-Container
- [ ] **OPS-02**: Konfiguration — Alle Parameter über Umgebungsvariablen oder YAML-Konfigurationsdatei
- [ ] **OPS-03**: Strukturiertes JSON-Logging — Konfigurierbar mit Log-Level, Indexierungsfortschritt und Fehlern
- [ ] **OPS-04**: Graceful Shutdown — Sauberes Herunterfahren mit Abschluss laufender Batch-Operationen
- [ ] **OPS-05**: Idempotenz — Wiederholte Indexierung desselben Dokuments führt zum gleichen Ergebnis (Upsert-Semantik)
- [ ] **OPS-06**: Fehlertoleranz — Temporäre Ausfälle führen zu Retry mit exponentiellem Backoff

### Infrastruktur

- [ ] **INFRA-01**: Embedder Interface — Austauschbare Embedding-Komponente hinter einheitlicher Go-Schnittstelle (lokal oder Cloud)
- [ ] **INFRA-02**: Qdrant Collection Setup — Collection mit Payload-Indizes für alle Filter-Dimensionen, deterministische Punkt-IDs
- [ ] **INFRA-03**: Embedding Model Benchmark — DE/EN Recall-Benchmark mit echten Daten vor Produktiv-Einsatz (multilingual-e5-base als Default)

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Indexierung

- **IDX-V2-01**: Dokument-Indexierung — Anhänge (PDF, DOCX, TXT, ODT) via Apache Tika Textextraktion
- **IDX-V2-02**: Chunk-level Retrieval — Passagen-genaue Ergebnisse mit Parent-Deduplication für lange Dokumente

### Suche

- **SRCH-V2-01**: Autocomplete/Suggest — Vorschläge während der Eingabe auf Basis partieller Suche

### Betrieb

- **OPS-V2-01**: Prometheus-kompatible Metriken — Indexierungsrate, Suchlatenz, Queue-Tiefe, Fehlerrate
- **OPS-V2-02**: Berechtigungs-Caching — Projektmitgliedschaften mit konfigurierbarer TTL gecacht

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Redmine Plugin (Ruby) | API-first Ansatz gewählt; Plugin-Entwicklung erfordert Ruby-Expertise und Redmine-Version-Kopplung |
| Web UI / Frontend | RSS ist Infrastruktur; Frontend-Integration über API durch Konsumenten |
| LLM Answer Generation (RAG) | Retrieval-Infrastruktur, keine Generierung; Halluzinationsrisiko und LLM-Abhängigkeit |
| Webhook-basierter Sync | Redmine hat keine nativen Webhooks; Polling robuster für MVP |
| Cross-Redmine Federation | Einzelinstanz zuerst; Multi-Tenant-Erweiterung über Collection-Naming möglich |
| Kubernetes Deployment | Docker Compose für v1 ausreichend; Kubernetes bei Bedarf in v2+ |
| Cross-Encoder Re-Ranking | Erst nach Validierung der Bi-Encoder-Qualität sinnvoll |
| User-personalisiertes Ranking | Erfordert Click-Tracking und MLOps-Stack; weit über Scope hinaus |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| IDX-01 | — | Pending |
| IDX-02 | — | Pending |
| IDX-03 | — | Pending |
| IDX-04 | — | Pending |
| IDX-05 | — | Pending |
| IDX-06 | — | Pending |
| IDX-07 | — | Pending |
| SRCH-01 | — | Pending |
| SRCH-02 | — | Pending |
| SRCH-03 | — | Pending |
| SRCH-04 | — | Pending |
| SRCH-05 | — | Pending |
| SRCH-06 | — | Pending |
| AUTH-01 | — | Pending |
| AUTH-02 | — | Pending |
| AUTH-03 | — | Pending |
| API-01 | — | Pending |
| API-02 | — | Pending |
| API-03 | — | Pending |
| API-04 | — | Pending |
| API-05 | — | Pending |
| OPS-01 | — | Pending |
| OPS-02 | — | Pending |
| OPS-03 | — | Pending |
| OPS-04 | — | Pending |
| OPS-05 | — | Pending |
| OPS-06 | — | Pending |
| INFRA-01 | — | Pending |
| INFRA-02 | — | Pending |
| INFRA-03 | — | Pending |

**Coverage:**
- v1 requirements: 30 total
- Mapped to phases: 0
- Unmapped: 30

---
*Requirements defined: 2026-02-18*
*Last updated: 2026-02-18 after initial definition*
