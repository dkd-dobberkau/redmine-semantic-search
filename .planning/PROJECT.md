# Redmine Semantic Search (RSS)

## What This Is

Eine semantische Suchinfrastruktur für Redmine, die es Nutzern ermöglicht, Inhalte (Issues, Wikis, Dokumente, Journale) über Bedeutung statt exakte Zeichenketten zu finden. Bestehend aus einem Go-Indexer, der Redmine-Inhalte vektorisiert und in Qdrant ablegt, sowie einer Such-API, die Anfragen entgegennimmt und gegen den Index abgleicht — unter vollständiger Einhaltung des Redmine-Berechtigungsmodells.

## Core Value

Nutzer finden relevante Redmine-Inhalte über semantische Suche, auch wenn sie die exakte Formulierung nicht kennen — ohne das Berechtigungsmodell zu umgehen.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Issue-Indexierung (Titel, Beschreibung, Journals, Custom Fields)
- [ ] Wiki-Indexierung mit Textile/Markdown-zu-Plaintext-Konvertierung
- [ ] Dokument-Indexierung via Textextraktion (PDF, DOCX, TXT, ODT)
- [ ] Journal-Indexierung als eigenständige Vektoren
- [ ] Inkrementelle Updates über `updated_on`-Feld
- [ ] Full Reindex ohne Suchunterbrechung (Blue-Green/Alias)
- [ ] Löschsynchronisation (gelöschte Objekte aus Index entfernen)
- [ ] Semantische Suche mit Cosine Similarity
- [ ] Hybrid Search (Vektor + Sparse/BM25)
- [ ] Facettierte Filter (Projekt, Tracker, Status, Autor, Zeitraum, Content-Typ)
- [ ] Paginierung (Default: 20, Max: 100)
- [ ] Snippet-Generierung mit relevantesten Passagen
- [ ] Ähnliche Issues (More Like This)
- [ ] Berechtigungsprüfung gegen Redmine-Projektmitgliedschaften
- [ ] Pre-Filtering via erlaubte `project_ids` als Qdrant-Filter
- [ ] API-Authentifizierung über Redmine API-Keys/Session-Tokens
- [ ] Berechtigungs-Caching mit konfigurierbarer TTL
- [ ] REST API (`GET /search`, `GET /similar`, `GET /health`, `POST /admin/reindex`)
- [ ] OpenAPI 3.x-Spezifikation
- [ ] Docker-Compose-Deployment (Qdrant, Embedding, Tika, Indexer)
- [ ] Prometheus-kompatible Metriken
- [ ] Strukturiertes JSON-Logging
- [ ] Graceful Shutdown

### Out of Scope

- Autocomplete/Suggest — Kann-Anforderung, auf v2+ verschoben
- Redmine-Plugin (Ruby) — Separater Service-Ansatz gewählt, kein Plugin-Entwicklung
- Kubernetes-Deployment — Docker Compose für v1, Kubernetes bei Bedarf später
- Eigene Web-UI — API-first, Frontend-Integration separat

## Context

- Redmine bietet nur textbasierte Suche, die bei wachsendem Datenbestand versagt
- Heterogene Inhalte: Issues, Wikis, Dokumente, Journale mit unterschiedlichen Strukturen
- Redmine hat standardmäßig keine Webhooks — Polling-basierter Ansatz für MVP
- Embedding-Modell austauschbar: lokal (MiniLM, multilingual-e5) oder Cloud (OpenAI)
- Inhalte primär deutsch und englisch — mehrsprachiges Modell empfohlen
- Datenschutz: Bei sensiblen Daten lokales Embedding bevorzugt
- Performance-Ziele: <200ms Suchantwort (P95), >=100 Docs/s Indexierung, <50ms Embedding-Latenz
- Zielgröße: bis zu 500.000 Vektoren

## Constraints

- **Tech Stack**: Go 1.22+, Qdrant (gRPC), Apache Tika als Sidecar — vorgegeben im Requirements-Dokument
- **Embedding**: Austauschbar hinter `Embedder`-Interface (lokal oder Cloud)
- **Berechtigungen**: Redmine-Berechtigungsmodell muss vollständig respektiert werden
- **Deployment**: Docker-Container für alle Komponenten
- **Idempotenz**: Wiederholte Indexierung muss identische Ergebnisse liefern (Upsert-Semantik)
- **Fehlertoleranz**: Retry mit exponentiellem Backoff bei temporären Ausfällen

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go als Sprache | Hoher Durchsatz, Single Binary, exzellente Concurrency | — Pending |
| Qdrant als Vektordatenbank | Payload-Indizes für gefilterte Suchen, benannte Vektoren für Hybrid Search | — Pending |
| Polling statt Webhooks | Redmine hat keine nativen Webhooks, Polling robuster für MVP | — Pending |
| Apache Tika für Textextraktion | Bewährt für PDF/DOCX/ODT, einfach als Sidecar-Container | — Pending |
| Chunking mit Überlappung | 512 Tokens Chunk-Größe, 50 Tokens Overlap für lange Dokumente | — Pending |
| Pre-Filtering Berechtigungen | project_ids als Qdrant-Filter für Performance und Sicherheit | — Pending |

---
*Last updated: 2026-02-18 after initialization*
