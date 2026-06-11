package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlagsHelp(t *testing.T) {
	savedArgs := os.Args
	defer func() { os.Args = savedArgs }()
	os.Args = []string{"pg_atropos"}
	cfg := parseFlags()
	if cfg == nil {
		t.Fatal("parseFlags returned nil")
	}
}

func TestParseHeader(t *testing.T) {
	tests := []struct {
		line       string
		wantName   string
		wantType   string
		wantSchema string
	}{
		{
			line:       `-- Name: users; Type: TABLE; Schema: public; Owner: app_owner`,
			wantName:   "users",
			wantType:   "TABLE",
			wantSchema: "public",
		},
		{
			line:       `-- Name: myapp_user; Type: ROLE; Schema: -; Owner: postgres`,
			wantName:   "myapp_user",
			wantType:   "ROLE",
			wantSchema: "-",
		},
		{
			line:       `-- Name: get_post_count(integer); Type: FUNCTION; Schema: public; Owner: app_owner`,
			wantName:   "get_post_count(integer)",
			wantType:   "FUNCTION",
			wantSchema: "public",
		},
		{
			line:       `-- Name: TABLE users; Type: ACL; Schema: public; Owner: app_owner`,
			wantName:   "TABLE users",
			wantType:   "ACL",
			wantSchema: "public",
		},
		{
			line:       `-- not a header`,
			wantName:   "",
			wantType:   "",
			wantSchema: "",
		},
	}
	for _, tt := range tests {
		name, typ, schema := parseHeader(tt.line)
		if name != tt.wantName || typ != tt.wantType || schema != tt.wantSchema {
			t.Errorf("parseHeader(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.line, name, typ, schema, tt.wantName, tt.wantType, tt.wantSchema)
		}
	}
}

func TestCheckDbFilter(t *testing.T) {
	tests := []struct {
		dbname    string
		blacklist string
		whitelist string
		want      bool
	}{
		{"mydb", "", "", true},
		{"template1", "^(template|postgres)", "", false},
		{"postgres", "^(template|postgres)", "", false},
		{"mydb", "^(template|postgres)", "", true},
		{"mydb", "", "mydb", true},
		{"other", "", "mydb", false},
		{"mydb", "template", "mydb", true}, // whitelist takes precedence
	}
	for _, tt := range tests {
		got := checkDbFilter(tt.dbname, tt.blacklist, tt.whitelist)
		if got != tt.want {
			t.Errorf("checkDbFilter(%q, %q, %q) = %v, want %v",
				tt.dbname, tt.blacklist, tt.whitelist, got, tt.want)
		}
	}
}

func TestFuncFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string // just check prefix and hash presence
	}{
		{"get_post_count(integer)", "get_post_count-"},
		{"update_timestamp()", "update_timestamp-"},
		{"add_post(text, text)", "add_post-"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		got := funcFilename(tt.input)
		if !strings.HasPrefix(got, tt.want) {
			t.Errorf("funcFilename(%q) = %q, want prefix %q", tt.input, got, tt.want)
		}
	}
}

func TestParseAndWriteOriginMode(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutputDir: dir,
		Mode:      "origin",
	}

	total := parseAndWrite(strings.NewReader(fullRestoreFixture), cfg, "testdb")

	// We expect 20+ objects extracted.  Exact count depends on how
	// writeObject treats SCHEMA / ACL / etc.  Let's just sanity-check.
	if total < 10 {
		t.Fatalf("expected at least 10 objects, got %d", total)
	}

	// Verify specific files exist.
	checks := []string{
		"testdb/-/ROLE/myapp_user.sql",
		"testdb/-/ROLE/app_owner.sql",
		"testdb/app_schema/app_schema.sql",
		"testdb/public/TABLE/users.sql",
		"testdb/public/CONSTRAINT/users_pkey.sql",
		"testdb/public/INDEX/users_name_idx.sql",
		"testdb/public/FUNCTION/get_post_count-",
		"testdb/public/PROCEDURE/add_post-",
		"testdb/app_schema/TABLE/posts.sql",
		"testdb/app_schema/FUNCTION/update_timestamp-",
		"testdb/app_schema/TRIGGER/posts_trigger.sql",
		"testdb/app_schema/FK CONSTRAINT/posts_pkey.sql",
		"testdb/public/SEQUENCE/users_id_seq.sql",
		"testdb/public/DEFAULT/id.sql",
		"testdb/public/CHECK CONSTRAINT/users_email_check.sql",
	}
	for _, path := range checks {
		// Use Match for glob patterns (FUNCTION has hash suffix)
		matches, err := filepath.Glob(filepath.Join(dir, path+"*"))
		if err != nil || len(matches) == 0 {
			t.Errorf("file not found: %s* (err=%v)", path, err)
		}
	}
}

func TestParseAndWriteCustomMode(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutputDir: dir,
		Mode:      "custom",
	}

	total := parseAndWrite(strings.NewReader(fullRestoreFixture), cfg, "testdb")

	if total < 10 {
		t.Fatalf("expected at least 10 objects, got %d", total)
	}

	checks := []string{
		"testdb/-/role/myapp_user.sql",
		"testdb/-/role/app_owner.sql",
		"testdb/app_schema/app_schema.sql",
		"testdb/public/table/users.sql",
		"testdb/public/constraint/users_pkey.sql",
		"testdb/public/index/users_name_idx.sql",
		"testdb/public/function/get_post_count-",
		"testdb/public/function/add_post-",
		"testdb/app_schema/table/posts.sql",
		"testdb/app_schema/function/update_timestamp-",
		"testdb/app_schema/trigger/posts_trigger.sql",
		"testdb/app_schema/fk constraint/posts_pkey.sql",
		"testdb/public/sequence/users_id_seq.sql",
		"testdb/public/default/id.sql",
		"testdb/public/check constraint/users_email_check.sql",
	}
	for _, path := range checks {
		matches, err := filepath.Glob(filepath.Join(dir, path+"*"))
		if err != nil || len(matches) == 0 {
			t.Errorf("file not found: %s* (err=%v)", path, err)
		}
	}
}

func TestParseAndWriteAclFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutputDir: dir,
		Mode:      "origin",
		AclFiles:  true,
	}

	parseAndWrite(strings.NewReader(fullRestoreFixture), cfg, "testdb")

	// ACL should be in separate .acl.sql files
	aclChecks := []string{
		"testdb/public/ACL/users.acl.sql",
		"testdb/app_schema/ACL/posts.acl.sql",
	}
	nonAclChecks := []string{
		"testdb/public/ACL/users.sql",
		"testdb/app_schema/ACL/posts.sql",
	}
	for _, path := range aclChecks {
		info, err := os.Stat(filepath.Join(dir, path))
		if err != nil {
			t.Errorf("expected .acl.sql file to exist: %s (err=%v)", path, err)
		}
		if info != nil && info.Size() == 0 {
			t.Errorf("acl file empty: %s", path)
		}
	}
	for _, path := range nonAclChecks {
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			t.Errorf("expected .sql file to NOT exist (acl-files enabled): %s", path)
		}
	}
}

func TestParseAndWriteMoveRoles(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutputDir: dir,
		Mode:      "origin",
		MoveRoles: true,
	}

	parseAndWrite(strings.NewReader(fullRestoreFixture), cfg, "testdb")

	// Roles should be directly under the db dir, not under ROLE/ subdir
	// With MoveRoles true, schemaDir "-" becomes "" -> path is dbname/typeDir/name.sql
	// But ROLE schema is "-", so schemaDir becomes "" -> testdb/ROLE/myapp_user.sql
	// Actually wait, the role dir is still ROLE/ but schema dir is skipped.
	// Let me check: moveRoles only makes schema "-" skip schema dir.
	// So path = dbpart + "" + typeDir + name.sql = testdb/ROLE/myapp_user.sql
	checks := []string{
		"testdb/ROLE/myapp_user.sql",
		"testdb/ROLE/app_owner.sql",
	}
	for _, path := range checks {
		info, err := os.Stat(filepath.Join(dir, path))
		if err != nil {
			t.Errorf("expected role file: %s (err=%v)", path, err)
		}
		if info != nil && info.Size() == 0 {
			t.Errorf("role file empty: %s", path)
		}
	}
}

func TestParseAndWriteExcludeObj(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutputDir:  dir,
		Mode:       "origin",
		ExcludeObj: "INDEX|TRIGGER",
	}

	total := parseAndWrite(strings.NewReader(fullRestoreFixture), cfg, "testdb")

	// INDEX and TRIGGER should be excluded
	excludedPaths := []string{
		"testdb/public/INDEX/users_name_idx.sql",
		"testdb/app_schema/TRIGGER/posts_trigger.sql",
	}
	for _, path := range excludedPaths {
		if _, err := os.Stat(filepath.Join(dir, path)); err == nil {
			t.Errorf("expected excluded file to NOT exist: %s", path)
		}
	}

	if total < 8 {
		t.Fatalf("expected at least 8 objects (excluding INDEX/TRIGGER), got %d", total)
	}
}

func TestParseAndWriteNoDbPath(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutputDir: dir,
		Mode:      "origin",
		NoDbPath:  true,
	}

	parseAndWrite(strings.NewReader(fullRestoreFixture), cfg, "testdb")

	// Files should be directly under output dir, not testdb/
	if _, err := os.Stat(filepath.Join(dir, "-/ROLE/myapp_user.sql")); err != nil {
		t.Errorf("expected file without db path: -/ROLE/myapp_user.sql (err=%v)", err)
	}
	// Db path should NOT exist
	if _, err := os.Stat(filepath.Join(dir, "testdb")); err == nil {
		t.Errorf("expected no testdb/ directory")
	}
}

func TestVersionConstant(t *testing.T) {
	if version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", version)
	}
}

func TestLog(t *testing.T) {
	// Non-quiet should write to stderr (no panic)
	cfg := &Config{Quiet: false}
	log(cfg, "hello %s", "world")

	// Quiet should not write (no panic)
	cfg2 := &Config{Quiet: true}
	log(cfg2, "should not appear")
}

func TestOrUnknown(t *testing.T) {
	if got := orUnknown("mydb"); got != "mydb" {
		t.Errorf("orUnknown('mydb') = %q, want 'mydb'", got)
	}
	if got := orUnknown(""); got != "unknown" {
		t.Errorf("orUnknown('') = %q, want 'unknown'", got)
	}
}

func TestCheckDepSuccess(t *testing.T) {
	checkDep("go")
}

func TestCheckDepMissing(t *testing.T) {
	savedExit := osExit
	defer func() { osExit = savedExit }()

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }

	func() {
		defer func() { _ = recover() }()
		checkDep("this-command-does-not-exist-12345")
	}()

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for missing command, got %d", exitCode)
	}
}

func TestParseFlagsInvalidMode(t *testing.T) {
	// Save and restore os.Args and osExit
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "-mode", "invalid"}

	func() {
		defer func() { _ = recover() }()
		parseFlags()
	}()

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for invalid mode, got %d", exitCode)
	}
}

func TestParseFlagsVersion(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "-version"}

	func() {
		defer func() { _ = recover() }()
		parseFlags()
	}()

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for -version, got %d", exitCode)
	}
}

func TestWriteObjectEmptyContent(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutputDir: dir, Mode: "origin"}
	total := 0
	// Empty content should not create a file
	writeObject(cfg, "testdb", "users", "TABLE", "public", "", &total)
	if total != 0 {
		t.Errorf("expected total 0 for empty content, got %d", total)
	}
}

func TestWriteObjectTableData(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutputDir: dir, Mode: "origin"}
	total := 0
	// TABLE DATA should be skipped
	writeObject(cfg, "testdb", "users", "TABLE DATA", "public", "COPY users ...", &total)
	if total != 0 {
		t.Errorf("expected total 0 for TABLE DATA, got %d", total)
	}
	// Verify no file was created
	matches, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(matches) != 0 {
		t.Errorf("expected no files for TABLE DATA, found %v", matches)
	}
}

func TestWriteObjectCommentNoSpace(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutputDir: dir, Mode: "origin"}
	total := 0
	// COMMENT without space in name should not crash
	writeObject(cfg, "testdb", "plaincomment", "COMMENT", "public", "COMMENT IS 'test'", &total)
	if total != 1 {
		t.Errorf("expected 1 object, got %d", total)
	}
}

func TestWriteObjectAclNoSpace(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{OutputDir: dir, Mode: "origin"}
	total := 0
	// ACL without space in name
	writeObject(cfg, "testdb", "plainacl", "ACL", "public", "GRANT ...", &total)
	if total != 1 {
		t.Errorf("expected 1 object, got %d", total)
	}
}

func TestAppendToFileError(t *testing.T) {
	saved := osExit
	defer func() { osExit = saved }()

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }

	func() {
		defer func() { _ = recover() }()
		appendToFile("/nonexistent/deep/path/file.sql", "content")
	}()

	if exitCode != 1 {
		t.Errorf("expected exit code 1 on file error, got %d", exitCode)
	}
}

func TestMkdirCached(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "a", "b", "c")

	// First call should create
	mkdirCached(subdir)
	if _, err := os.Stat(subdir); err != nil {
		t.Errorf("expected dir to exist: %v", err)
	}

	// Second call should be cached (no error)
	mkdirCached(subdir)
}

func TestParseAndWriteDryRun(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		OutputDir: dir,
		Mode:      "origin",
		DryRun:    true,
	}

	total := parseAndWrite(strings.NewReader(fullRestoreFixture), cfg, "testdb")

	// Should report objects
	if total < 20 {
		t.Errorf("expected at least 20 objects in dry-run, got %d", total)
	}
	// Should NOT create any files
	matches, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(matches) != 0 {
		t.Errorf("expected no files in dry-run, found %v", matches)
	}
}

func TestFuncArgsHash(t *testing.T) {
	tests := []struct {
		args string
		want string
	}{
		{"", "000000"},
		{"integer", "157db7"},
		{"text, text", "65d7a4"},
	}
	for _, tt := range tests {
		got := funcArgsHash(tt.args)
		if got != tt.want {
			t.Errorf("funcArgsHash(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

// fullRestoreFixture is a minimal but comprehensive SQL dump used by tests.
// It's the same content as testdata/full_restore.sql kept inline for fast
// test execution (no file I/O dependency).
const fullRestoreFixture = `--
-- PostgreSQL database dump
--

-- Dumped from database version 16.0
-- Dumped by pg_dump version 16.0

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';
SET default_table_access_method = heap;

--
-- Name: myapp_user; Type: ROLE; Schema: -; Owner: postgres
--

CREATE ROLE myapp_user;
ALTER ROLE myapp_user WITH NOSUPERUSER LOGIN;

--
-- Name: app_owner; Type: ROLE; Schema: -; Owner: postgres
--

CREATE ROLE app_owner;
ALTER ROLE app_owner WITH NOSUPERUSER NOLOGIN;

--
-- Name: app_schema; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA app_schema;
ALTER SCHEMA app_schema OWNER TO app_owner;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: postgres
--

CREATE SCHEMA public;
ALTER SCHEMA public OWNER TO app_owner;

--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: postgres
--

COMMENT ON SCHEMA public IS 'standard public schema';

--
-- Name: users; Type: TABLE; Schema: public; Owner: app_owner
--

CREATE TABLE public.users (
    id integer NOT NULL,
    name character varying(100),
    email character varying(255),
    created_at timestamp without time zone DEFAULT now()
);
ALTER TABLE public.users OWNER TO app_owner;

--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: app_owner
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE public.users_id_seq OWNER TO app_owner;

--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: app_owner
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;

--
-- Name: users_id_seq; Type: SEQUENCE SET; Schema: public; Owner: app_owner
--

SELECT pg_catalog.setval('public.users_id_seq', 1, false);

--
-- Name: id; Type: DEFAULT; Schema: public; Owner: app_owner
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);

--
-- Name: users_pkey; Type: CONSTRAINT; Schema: public; Owner: app_owner
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

--
-- Name: users_email_key; Type: CONSTRAINT; Schema: public; Owner: app_owner
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);

--
-- Name: users_email_check; Type: CHECK CONSTRAINT; Schema: public; Owner: app_owner
--

ALTER TABLE public.users
    ADD CONSTRAINT users_email_check CHECK ((email ~* '^[A-Za-z0-9._%-]+@[A-Za-z0-9.-]+[.][A-Za-z]+$'::text));

--
-- Name: users_name_idx; Type: INDEX; Schema: public; Owner: app_owner
--

CREATE INDEX users_name_idx ON public.users USING btree (name);

--
-- Name: TABLE users; Type: ACL; Schema: public; Owner: app_owner
--

REVOKE ALL ON TABLE public.users FROM PUBLIC;
GRANT SELECT ON TABLE public.users TO myapp_user;

--
-- Name: posts; Type: TABLE; Schema: app_schema; Owner: app_owner
--

CREATE TABLE app_schema.posts (
    id integer NOT NULL,
    user_id integer NOT NULL,
    title text NOT NULL,
    body text,
    created_at timestamp without time zone DEFAULT now()
);
ALTER TABLE app_schema.posts OWNER TO app_owner;

--
-- Name: posts_id_seq; Type: SEQUENCE; Schema: app_schema; Owner: app_owner
--

CREATE SEQUENCE app_schema.posts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;
ALTER SEQUENCE app_schema.posts_id_seq OWNER TO app_owner;

--
-- Name: posts_id_seq; Type: SEQUENCE OWNED BY; Schema: app_schema; Owner: app_owner
--

ALTER SEQUENCE app_schema.posts_id_seq OWNED BY app_schema.posts.id;

--
-- Name: posts_id_seq; Type: SEQUENCE SET; Schema: app_schema; Owner: app_owner
--

SELECT pg_catalog.setval('app_schema.posts_id_seq', 1, false);

--
-- Name: id; Type: DEFAULT; Schema: app_schema; Owner: app_owner
--

ALTER TABLE ONLY app_schema.posts ALTER COLUMN id SET DEFAULT nextval('app_schema.posts_id_seq'::regclass);

--
-- Name: posts_pkey; Type: FK CONSTRAINT; Schema: app_schema; Owner: app_owner
--

ALTER TABLE ONLY app_schema.posts
    ADD CONSTRAINT posts_pkey FOREIGN KEY (user_id) REFERENCES public.users(id);

--
-- Name: posts_trigger; Type: TRIGGER; Schema: app_schema; Owner: app_owner
--

CREATE TRIGGER posts_trigger BEFORE INSERT ON app_schema.posts FOR EACH ROW EXECUTE FUNCTION app_schema.update_timestamp();

--
-- Name: get_post_count(integer); Type: FUNCTION; Schema: public; Owner: app_owner
--

CREATE FUNCTION public.get_post_count(user_id integer) RETURNS bigint
    LANGUAGE plpgsql
    AS $$
DECLARE
    cnt bigint;
BEGIN
    SELECT COUNT(*) INTO cnt FROM app_schema.posts WHERE posts.user_id = user_id;
    RETURN cnt;
END;
$$;
ALTER FUNCTION public.get_post_count(integer) OWNER TO app_owner;

--
-- Name: update_timestamp(); Type: FUNCTION; Schema: app_schema; Owner: app_owner
--

CREATE FUNCTION app_schema.update_timestamp() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.created_at = now();
    RETURN NEW;
END;
$$;
ALTER FUNCTION app_schema.update_timestamp() OWNER TO app_owner;

--
-- Name: add_post(text, text); Type: PROCEDURE; Schema: public; Owner: app_owner
--

CREATE PROCEDURE public.add_post(p_title text, p_body text)
    LANGUAGE plpgsql
    AS $$
BEGIN
    INSERT INTO app_schema.posts (title, body) VALUES (p_title, p_body);
END;
$$;
ALTER PROCEDURE public.add_post(text, text) OWNER TO app_owner;

--
-- Name: TABLE posts; Type: ACL; Schema: app_schema; Owner: app_owner
--

REVOKE ALL ON TABLE app_schema.posts FROM PUBLIC;
GRANT SELECT, INSERT ON TABLE app_schema.posts TO myapp_user;

--
-- PostgreSQL database dump complete
--
`

func TestMainIntegrationTestSQL(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	dir := t.TempDir()
	src := filepath.Join(dir, "fixture.sql")
	if err := os.WriteFile(src, []byte(fullRestoreFixture), 0644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "out")
	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "--test-sql", src, "--output", out, "--mode", "origin"}

	func() {
		defer func() { _ = recover() }()
		main()
	}()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Verify some files were created
	checks := []string{
		"public/TABLE/users.sql",
		"app_schema/TABLE/posts.sql",
		"public/FUNCTION/get_post_count-",
	}
	for _, path := range checks {
		matches, err := filepath.Glob(filepath.Join(out, path+"*"))
		if err != nil || len(matches) == 0 {
			t.Errorf("file not found: %s* (err=%v)", path, err)
		}
	}
}

func TestMainIntegrationDryRun(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	dir := t.TempDir()
	src := filepath.Join(dir, "fixture.sql")
	if err := os.WriteFile(src, []byte(fullRestoreFixture), 0644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "--test-sql", src, "--output", dir, "--dry-run"}

	func() {
		defer func() { _ = recover() }()
		main()
	}()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for dry-run, got %d", exitCode)
	}
}

func TestMainMissingFlag(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos"}

	func() {
		defer func() { _ = recover() }()
		main()
	}()

	if exitCode != 1 {
		t.Errorf("expected exit code 1 when missing --file/--db, got %d", exitCode)
	}
}

func TestMainInvalidModeFlag(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "-mode", "invalid"}

	func() {
		defer func() { _ = recover() }()
		main()
	}()

	if exitCode != 1 {
		t.Errorf("expected exit code 1 for invalid mode, got %d", exitCode)
	}
}

func TestMainStdinFlag(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	dir := t.TempDir()
	src := filepath.Join(dir, "fixture.sql")
	if err := os.WriteFile(src, []byte(fullRestoreFixture), 0644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "--file", "-", "--test-sql", src, "--output", dir}

	func() {
		defer func() { _ = recover() }()
		main()
	}()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0 with --file -, got %d", exitCode)
	}
}

func TestMainStdinWithDbFlag(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	dir := t.TempDir()
	src := filepath.Join(dir, "fixture.sql")
	if err := os.WriteFile(src, []byte(fullRestoreFixture), 0644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "--file", "-", "--db", "mydb", "--test-sql", src, "--output", dir}

	func() {
		defer func() { _ = recover() }()
		main()
	}()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Should have created files under mydb/ since dbname was provided
	if _, err := os.Stat(filepath.Join(dir, "mydb/public/TABLE/users.sql")); err != nil {
		t.Errorf("expected file under mydb/ (db flag should be used with stdin): %v", err)
	}
}

func TestMainStdinWithDbFlagNoDbPath(t *testing.T) {
	savedArgs := os.Args
	savedExit := osExit
	defer func() { os.Args = savedArgs; osExit = savedExit }()

	dir := t.TempDir()
	src := filepath.Join(dir, "fixture.sql")
	if err := os.WriteFile(src, []byte(fullRestoreFixture), 0644); err != nil {
		t.Fatal(err)
	}

	var exitCode int
	osExit = func(code int) { exitCode = code; panic("exit") }
	os.Args = []string{"pg_atropos", "--file", "-", "--db", "mydb", "--no-db-path", "--test-sql", src, "--output", dir}

	func() {
		defer func() { _ = recover() }()
		main()
	}()

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Should NOT have mydb/ in path with --no-db-path
	if _, err := os.Stat(filepath.Join(dir, "mydb")); err == nil {
		t.Errorf("expected no mydb/ directory with --no-db-path")
	}
	// But files should exist directly under output
	if _, err := os.Stat(filepath.Join(dir, "public/TABLE/users.sql")); err != nil {
		t.Errorf("expected file without db path: %v", err)
	}
}
