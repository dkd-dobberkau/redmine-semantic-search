// Package main contains the Recall@10 benchmark for the multilingual-e5-base embedding model.
package main

// QAPair is a query-passage pair used to evaluate embedding recall.
// The Query simulates a Redmine search query; the Passage simulates the
// matching issue, wiki, or document text that should appear in top-K results.
//
// Passages must NOT include the "passage: " prefix — the Embedder handles that.
type QAPair struct {
	Query   string
	Passage string
}

// BenchmarkPairs contains 50+ synthetic DE/EN QA pairs that represent
// realistic Redmine content: bug reports, feature requests, support tickets,
// wiki pages, and journal entries. Pairs are used to compute Recall@10.
//
// Distribution:
//   - 20 German-language pairs (index 0-19)
//   - 20 English-language pairs (index 20-39)
//   - 10 cross-lingual / edge-case pairs (index 40-49)
var BenchmarkPairs = []QAPair{
	// --- German pairs (0-19) ---

	// Bug reports (DE)
	{
		Query:   "Fehler beim Upload von Anhängen",
		Passage: "Beim Hochladen von Anhängen größer als 10MB tritt ein Timeout-Fehler auf. Der Fehler betrifft alle Projekte und ist reproduzierbar mit Firefox und Chrome. Logs zeigen einen 504 Gateway Timeout nach genau 30 Sekunden.",
	},
	{
		Query:   "Anmeldung mit LDAP schlägt fehl",
		Passage: "Die Authentifizierung via LDAP schlägt fehl, wenn der Domain-Controller nicht erreichbar ist. Fehlermeldung: Verbindungstimeout nach 5 Sekunden. Betrifft alle Nutzer im Büro München. Workaround: lokale Anmeldung nutzen.",
	},
	{
		Query:   "E-Mail-Benachrichtigungen werden nicht verschickt",
		Passage: "E-Mail-Benachrichtigungen für neue Issues werden seit dem Update auf Version 5.1 nicht mehr verschickt. Die SMTP-Konfiguration ist korrekt, Testmails funktionieren. Das Problem tritt nur bei automatischen Benachrichtigungen auf.",
	},
	{
		Query:   "Gantt-Diagramm lädt nicht",
		Passage: "Das Gantt-Diagramm im Projekt 'Infrastruktur' zeigt nach dem Laden nur einen weißen Bildschirm. Die Browser-Konsole zeigt einen JavaScript-Fehler: TypeError: Cannot read property 'data' of undefined. Betroffen: alle Projekte mit mehr als 200 Aufgaben.",
	},
	{
		Query:   "Zeiterfassung speichert falsche Daten",
		Passage: "Beim Speichern von Zeiteinträgen werden die Stunden auf den falschen Issue gebucht. Das Problem tritt auf, wenn man über den Aktivitäts-Tab bucht. Reproduzierbar in Redmine 5.0.3, Datenbank PostgreSQL 14.",
	},
	{
		Query:   "PDF-Export enthält falsche Sonderzeichen",
		Passage: "Der PDF-Export von Wiki-Seiten wandelt Umlaute (ä, ö, ü, ß) in Fragezeichen um. Das Problem tritt bei allen Wiki-Seiten auf, die deutschen Text enthalten. HTML-Export funktioniert korrekt. Betroffen: PDF-Export, Druckansicht.",
	},
	{
		Query:   "API gibt falschen HTTP-Statuscode zurück",
		Passage: "Die Redmine REST API gibt bei ungültigen API-Keys den Statuscode 500 statt 401 zurück. Das erschwert die Fehlerbehandlung in Client-Anwendungen. Reproduzierbar mit curl und Postman. Betrifft alle API-Endpunkte.",
	},

	// Feature requests (DE)
	{
		Query:   "Benachrichtigung bei Fälligkeit von Aufgaben",
		Passage: "Feature-Request: Nutzer sollen eine E-Mail-Benachrichtigung erhalten, wenn eine Aufgabe in 3 Tagen fällig wird. Konfigurierbar pro Nutzer und Projekt. Ähnliche Funktionalität in Jira vorhanden. Priorität: mittel.",
	},
	{
		Query:   "Mehrsprachige Projektbeschreibungen",
		Passage: "Es soll möglich sein, Projektbeschreibungen in mehreren Sprachen zu hinterlegen. Der Nutzer sieht die Beschreibung in seiner eingestellten Sprache. Fallback auf Englisch falls die Übersetzung fehlt. Betrifft alle Projekte.",
	},
	{
		Query:   "Zwei-Faktor-Authentifizierung einrichten",
		Passage: "Feature-Anfrage: Redmine soll TOTP-basierte Zwei-Faktor-Authentifizierung unterstützen (z.B. Google Authenticator). Admin kann 2FA für alle Nutzer erzwingen. Backup-Codes sollen bereitgestellt werden.",
	},

	// Support tickets (DE)
	{
		Query:   "Datenbankverbindung nach Update unterbrochen",
		Passage: "Nach dem Update auf Redmine 5.1 gibt es Datenbankverbindungsfehler beim Start. Fehlermeldung: 'Could not connect to server: Connection refused'. PostgreSQL läuft und ist erreichbar. Firewall-Regeln wurden nicht geändert.",
	},
	{
		Query:   "Update auf Version 4.2 bricht Datenbankmigrationen",
		Passage: "Nach dem Update von Redmine 4.1 auf 4.2 schlägt die Datenbankmigration mit dem Fehler 'column already exists' fehl. Die Migration muss manuell rückgängig gemacht werden. Betroffen: PostgreSQL und MySQL. Workaround: migration_context.rollback ausführen.",
	},
	{
		Query:   "Redmine startet nicht nach Kernel-Update",
		Passage: "Nach einem Linux-Kernel-Update startet Redmine nicht mehr. Der Passenger-Prozess beendet sich sofort mit exit code 1. Logs zeigen: 'Failed to load application'. Ruby und Bundler sind korrekt installiert. Betroffen: Ubuntu 22.04.",
	},
	{
		Query:   "Benutzer kann eigenes Passwort nicht ändern",
		Passage: "Reguläre Nutzer können ihr Passwort nicht über das Profil ändern. Das Formular speichert die Änderung, aber das alte Passwort funktioniert weiterhin. Admin-Passwortänderungen funktionieren. Betroffen: Redmine 5.0 auf Rails 6.",
	},

	// Wiki / technical content (DE)
	{
		Query:   "Backup und Wiederherstellung Anleitung",
		Passage: "Diese Anleitung beschreibt das Backup und die Wiederherstellung von Redmine. Sicherung der Datenbank: pg_dump -Fc redmine > backup.dump. Sicherung der Anhänge: tar -czf attachments.tar.gz /var/redmine/files. Wiederherstellung: dropdb redmine && createdb redmine && pg_restore -d redmine backup.dump.",
	},
	{
		Query:   "Konfiguration des SMTP-Servers",
		Passage: "Die SMTP-Konfiguration erfolgt in config/configuration.yml. Pflichtfelder: address, port, user_name, password, authentication. TLS aktivieren: enable_starttls_auto: true. Nach Änderungen muss Redmine neu gestartet werden.",
	},
	{
		Query:   "Performance-Optimierung für große Projekte",
		Passage: "Für Projekte mit mehr als 10.000 Issues empfehlen wir folgende Optimierungen: Aktivierung von pg_stat_statements für Query-Analyse, Erhöhung von work_mem auf 64MB, Erstellung von Datenbankindizes auf issues.assigned_to_id und issues.project_id.",
	},
	{
		Query:   "Plugin-Installation und -Konfiguration",
		Passage: "Plugins werden im Verzeichnis plugins/ abgelegt. Installation: bundle install und rake redmine:plugins:migrate RAILS_ENV=production. Deinstallation: rake redmine:plugins:migrate NAME=plugin_name VERSION=0 RAILS_ENV=production. Anschließend Redmine neu starten.",
	},
	{
		Query:   "Rollen und Berechtigungen konfigurieren",
		Passage: "Rollen werden unter Administration > Rollen und Berechtigungen verwaltet. Jede Rolle hat granulare Berechtigungen für Issues, Wiki, Dokumente und Zeiterfassung. Projektmitglieder erhalten ihre Berechtigungen über die Rollenzuweisung im Projektbereich.",
	},
	{
		Query:   "Automatische Versionsverwaltung mit Git",
		Passage: "Redmine unterstützt die Integration mit Git-Repositories. Konfiguration unter Einstellungen > Versionsverwaltung. Das Repository wird periodisch aktualisiert (fetch_changesets). Commits können Issues über die Syntax 'Fixes #123' schließen.",
	},

	// --- English pairs (20-39) ---

	// Bug reports (EN)
	{
		Query:   "login fails with LDAP",
		Passage: "Authentication via LDAP fails when the domain controller is unreachable. Error: connection timeout after 5s. Affects all users in the EU office. Workaround: use local authentication until the DC is restored.",
	},
	{
		Query:   "email notifications not sent after upgrade",
		Passage: "Email notifications for new issues stopped being sent after upgrading to version 5.1. SMTP configuration is correct, test emails work fine. The issue only affects automatic notifications triggered by issue creation and updates.",
	},
	{
		Query:   "file attachment upload fails",
		Passage: "Uploading file attachments larger than 10MB fails with a 504 Gateway Timeout error. This affects all projects and is reproducible with both Firefox and Chrome. Server logs show the timeout occurs at exactly 30 seconds.",
	},
	{
		Query:   "Gantt chart not loading",
		Passage: "The Gantt chart for the 'Infrastructure' project shows only a white screen after loading. Browser console shows TypeError: Cannot read property 'data' of undefined. Affects all projects with more than 200 tasks.",
	},
	{
		Query:   "NullPointerException on PDF export",
		Passage: "PDF export of wiki pages throws a NullPointerException in the PDF generation library. Stack trace shows the error occurs in WikiPDFExport.rb line 47. Only affects wiki pages containing tables with merged cells.",
	},
	{
		Query:   "time tracking hours booked to wrong issue",
		Passage: "When logging time via the Activity tab, hours are booked to the wrong issue. The bug is reproducible in Redmine 5.0.3 with PostgreSQL 14. Time entries created via the issue detail view work correctly.",
	},
	{
		Query:   "REST API returns 500 instead of 401",
		Passage: "The Redmine REST API returns HTTP 500 instead of 401 for invalid API keys. This makes error handling difficult in client applications. Reproducible with curl and Postman. Affects all API endpoints.",
	},

	// Feature requests (EN)
	{
		Query:   "email notification before task due date",
		Passage: "Feature request: users should receive an email notification 3 days before a task is due. Configurable per user and per project. Similar functionality exists in Jira. Priority: medium. Assignee: platform team.",
	},
	{
		Query:   "two-factor authentication support",
		Passage: "Feature request: Redmine should support TOTP-based two-factor authentication (e.g., Google Authenticator). Admins can enforce 2FA for all users. Backup codes should be provided for account recovery.",
	},
	{
		Query:   "custom fields in issue list export",
		Passage: "When exporting the issue list to CSV or XLS, custom fields are not included in the export. Request: add custom fields to the export. The current workaround is to use the API to fetch custom field values separately.",
	},

	// Support tickets (EN)
	{
		Query:   "database connection error after update",
		Passage: "After updating to Redmine 5.1, database connection errors appear on startup. Error message: 'Could not connect to server: Connection refused'. PostgreSQL is running and accessible. Firewall rules have not been changed.",
	},
	{
		Query:   "migration fails with column already exists",
		Passage: "After upgrading from Redmine 4.1 to 4.2, the database migration fails with 'column already exists'. The migration must be rolled back manually. Affects both PostgreSQL and MySQL. Workaround: run migration_context.rollback.",
	},
	{
		Query:   "user cannot change their own password",
		Passage: "Regular users cannot change their password through the profile page. The form saves but the old password continues to work. Admin password changes work correctly. Affects Redmine 5.0 running on Rails 6.",
	},
	{
		Query:   "plugin installation breaks existing functionality",
		Passage: "Installing the redmine_agile plugin causes the issue list page to throw a NoMethodError. The error appears in the view layer when the plugin hooks into the issue list. Removing the plugin restores functionality. Plugin version: 1.6.2.",
	},

	// Wiki / technical content (EN)
	{
		Query:   "backup and restore procedure",
		Passage: "This guide describes the Redmine backup and restore procedure. Database backup: pg_dump -Fc redmine > backup.dump. Attachment backup: tar -czf attachments.tar.gz /var/redmine/files. Restore: dropdb redmine && createdb redmine && pg_restore -d redmine backup.dump.",
	},
	{
		Query:   "SMTP configuration for email delivery",
		Passage: "SMTP configuration is set in config/configuration.yml. Required fields: address, port, user_name, password, authentication. Enable TLS: enable_starttls_auto: true. Redmine must be restarted after changing the configuration.",
	},
	{
		Query:   "performance tuning for large projects",
		Passage: "For projects with more than 10,000 issues we recommend: enable pg_stat_statements for query analysis, increase work_mem to 64MB, create database indexes on issues.assigned_to_id and issues.project_id. Monitor with EXPLAIN ANALYZE.",
	},
	{
		Query:   "role and permission configuration",
		Passage: "Roles are managed under Administration > Roles and Permissions. Each role has granular permissions for issues, wiki, documents, and time tracking. Project members receive permissions through role assignments in the project members section.",
	},
	{
		Query:   "Git repository integration setup",
		Passage: "Redmine supports Git repository integration. Configuration under Settings > Repositories. The repository is updated periodically via fetch_changesets. Commits can close issues using the syntax 'Fixes #123' in commit messages.",
	},
	{
		Query:   "API rate limiting and authentication",
		Passage: "The Redmine REST API supports key-based authentication. Pass the API key in the X-Redmine-API-Key header or as the key query parameter. Rate limiting is not built in — use a reverse proxy (e.g., nginx) to apply per-IP limits.",
	},

	// --- Cross-lingual / edge-case pairs (40-49) ---

	// Short technical queries
	{
		Query:   "NullPointerException bei PDF-Export",
		Passage: "PDF export of wiki pages throws a NullPointerException in the PDF generation library. The error occurs in WikiPDFExport.rb line 47 when processing tables with merged cells. No fix available yet; avoid merged cells as a workaround.",
	},
	{
		Query:   "500 error API key",
		Passage: "Die Redmine REST API gibt bei ungültigen API-Keys den Statuscode 500 statt 401 zurück. Das erschwert die Fehlerbehandlung in Client-Anwendungen erheblich.",
	},
	// Queries with version numbers
	{
		Query:   "Update auf Version 4.2 bricht Plugin-Kompatibilität",
		Passage: "After upgrading from Redmine 4.1 to 4.2, several plugins stop working. The agile plugin throws a NoMethodError on the issue list page. Plugin authors need to update their hooks for the new view structure introduced in 4.2.",
	},
	{
		Query:   "Redmine 5.1 LDAP regression",
		Passage: "In Redmine 5.1 wurde die LDAP-Authentifizierung überarbeitet. Dabei wurde ein Fehler eingeführt: Nutzer mit Sonderzeichen im Benutzernamen können sich nicht mehr anmelden. Betroffen: Umlaute und Leerzeichen im DN.",
	},
	// Long structured wiki content
	{
		Query:   "Installationsanleitung Ubuntu",
		Passage: "Redmine Installation auf Ubuntu 22.04: 1) Ruby 3.1 via rbenv installieren. 2) PostgreSQL installieren: apt install postgresql. 3) Datenbank erstellen: createdb redmine. 4) Bundler: gem install bundler. 5) bundle install --without development test. 6) rake db:migrate RAILS_ENV=production. 7) Passenger oder Puma als Webserver konfigurieren.",
	},
	{
		Query:   "Docker deployment guide",
		Passage: "Redmine Docker-Deployment: Das offizielle Docker-Image liegt auf Docker Hub. Starten mit: docker run -d -p 3000:3000 redmine:latest. Daten persistieren: Volume für /usr/src/redmine/files und /usr/src/redmine/log einbinden. Datenbank als separaten Container oder externen Service betreiben.",
	},
	// Code-related
	{
		Query:   "rake task für Datenbereinigung",
		Passage: "A custom Rake task for cleaning up old journals: task :cleanup_journals => :environment do Journal.where('created_on < ?', 1.year.ago).delete_all end. Run with: rake cleanup_journals RAILS_ENV=production. Creates no backup — test in staging first.",
	},
	{
		Query:   "custom query API endpoint",
		Passage: "Mit der Redmine REST API können benutzerdefinierte Abfragen abgerufen werden. Endpunkt: GET /issues.json?query_id=42. Die query_id entspricht der ID der gespeicherten Abfrage im Browser. Unterstützt Paginierung über offset und limit Parameter.",
	},
	// Mixed language edge cases
	{
		Query:   "Webhook bei Issue-Statusänderung",
		Passage: "Redmine does not have built-in webhook support. The redmine_webhook plugin adds POST requests to a configurable URL on issue status changes, creation, and updates. Payload format is JSON with the full issue object.",
	},
	{
		Query:   "issue bulk update permission denied",
		Passage: "Beim Massenbearbeiten von Issues erhalten Nutzer eine 'Permission denied' Fehlermeldung, obwohl sie die Berechtigung 'Issues bearbeiten' haben. Das Problem tritt auf, wenn Issues aus verschiedenen Projekten gleichzeitig bearbeitet werden.",
	},
}
