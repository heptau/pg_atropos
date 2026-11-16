package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var version = "1.1.0"
var osExit = os.Exit

// tmpCleanup removes temporary files; it is run by exit() because osExit
// does not run deferred functions.
var tmpCleanup = func() {}

func exit(code int) {
	tmpCleanup()
	osExit(code)
}

// ---------- Config ----------

type Config struct {
	DBName      string
	Conn        string
	DumpFile    string
	OutputDir   string
	Mode        string
	Clean       bool
	NoDbPath    bool
	BlacklistDb string
	WhitelistDb string
	ExcludeObj  string
	AclFiles    bool
	MoveRoles   bool
	Quiet       bool
	DryRun      bool
	Version     bool
	TestSQL     string
}

// ---------- Logging ----------

func log(cfg *Config, format string, args ...interface{}) {
	if !cfg.Quiet {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

// ---------- Dependency check ----------

func checkDep(name string) {
	if _, err := exec.LookPath(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: required command not found: %s\n", name)
		exit(1)
	}
}

// ---------- Main ----------

func main() {
	cfg := parseFlags()

	if cfg.DryRun {
		fmt.Fprintln(os.Stderr, "Dry-run mode: no files will be written")
	}

	if cfg.DumpFile == "" && cfg.DBName == "" && cfg.Conn == "" && cfg.TestSQL == "" {
		fmt.Fprintln(os.Stderr, "Error: one of --file (or --file -), --db or --conn is required")
		exit(1)
	}

	dbname := cfg.DBName

	// Create dump if needed (skip for stdin and test-sql mode)
	isStdin := cfg.DumpFile == "-"
	dumpFile := cfg.DumpFile
	if cfg.TestSQL == "" && !isStdin && dumpFile == "" {
		checkDep("pg_dump")
		tmpDir, err := os.MkdirTemp("", "pg_atropos_*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating temp dir: %v\n", err)
			exit(1)
		}
		tmpCleanup = func() { _ = os.RemoveAll(tmpDir) }
		defer func() { tmpCleanup(); tmpCleanup = func() {} }()
		dumpFile = filepath.Join(tmpDir, "dump.pgdump")

		// pg_dump accepts a connection string or URI as the database name.
		target := cfg.DBName
		if cfg.Conn != "" {
			target = cfg.Conn
		}
		log(cfg, "Creating dump of database: %s", orUnknown(cfg.DBName))
		cmd := exec.Command("pg_dump", "-Fc", "-f", dumpFile, "-d", target)
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(os.Stderr, "pg_dump failed: %v\n%s\n", err, string(out))
			exit(1)
		}
		log(cfg, "Dump created: %s", dumpFile)
	}

	// Detect dbname from TOC if not provided (skip for stdin and test-sql mode)
	if dbname == "" && cfg.TestSQL == "" && !isStdin {
		dbname = detectDbname(dumpFile)
	}

	log(cfg, "Database: %s", orUnknown(dbname))
	log(cfg, "Mode: %s", cfg.Mode)

	// Check blacklist/whitelist
	if !checkDbFilter(dbname, cfg.BlacklistDb, cfg.WhitelistDb) {
		log(cfg, "Database '%s' is excluded by filter, skipping", dbname)
		return
	}

	if cfg.Clean && !cfg.DryRun {
		if err := cleanOutputDir(cfg.OutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to clean %s: %v\n", cfg.OutputDir, err)
			exit(1)
		}
		log(cfg, "Cleaned output directory: %s", cfg.OutputDir)
	}
	if !cfg.DryRun {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
			exit(1)
		}
	}

	// Source of SQL: pg_restore pipe or test SQL file
	var total int
	if cfg.TestSQL != "" {
		f, err := os.Open(cfg.TestSQL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening test SQL file: %v\n", err)
			exit(1)
		}
		total = parseAndWrite(f, cfg, dbname)
		_ = f.Close()
	} else {
		checkDep("pg_restore")
		args := []string{"-f", "-"}
		if !isStdin {
			args = append(args, dumpFile)
		}
		cmd := exec.Command("pg_restore", args...)
		if isStdin {
			cmd.Stdin = os.Stdin
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating pipe: %v\n", err)
			exit(1)
		}
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "pg_restore failed: %v\n", err)
			exit(1)
		}
		total = parseAndWrite(stdout, cfg, dbname)
		if err := cmd.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "pg_restore failed: %v\n", err)
			exit(1)
		}
	}

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "Dry-run: would extract %d objects to %s\n", total, cfg.OutputDir)
	} else {
		log(cfg, "Extracted %d objects to: %s", total, cfg.OutputDir)
		log(cfg, "Done.")
	}
}

// cleanOutputDir removes the output directory, refusing obviously
// dangerous targets (filesystem root, home directory, working directory).
func cleanOutputDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if abs == filepath.VolumeName(abs)+string(filepath.Separator) {
		return fmt.Errorf("refusing to remove filesystem root")
	}
	if home, err := os.UserHomeDir(); err == nil && abs == filepath.Clean(home) {
		return fmt.Errorf("refusing to remove home directory")
	}
	if wd, err := os.Getwd(); err == nil && abs == wd {
		return fmt.Errorf("refusing to remove current working directory")
	}
	return os.RemoveAll(abs)
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// ---------- Flag parsing ----------

func parseFlags() *Config {
	cfg := &Config{
		OutputDir:   "./output",
		Mode:        "origin",
		BlacklistDb: "^(template|postgres)",
	}

	// Extract hidden --test-sql before flag parsing so it doesn't appear in --help
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--test-sql" && i+1 < len(args) {
			cfg.TestSQL = args[i+1]
			args = append(args[:i], args[i+2:]...)
			break
		}
	}

	fs := flag.NewFlagSet("pg_atropos", flag.ContinueOnError)
	fs.StringVar(&cfg.DBName, "db", "", "Database name to dump")
	fs.StringVar(&cfg.DBName, "d", "", "Shorthand for --db")
	fs.StringVar(&cfg.Conn, "conn", "", "PostgreSQL connection string")
	fs.StringVar(&cfg.DumpFile, "file", "", "Custom-format dump file (\"-\" for stdin)")
	fs.StringVar(&cfg.DumpFile, "f", "", "Shorthand for --file (e.g. -f - for stdin)")
	fs.StringVar(&cfg.OutputDir, "output", "./output", "Output directory")
	fs.StringVar(&cfg.OutputDir, "o", "./output", "Shorthand for --output")
	fs.StringVar(&cfg.Mode, "mode", "origin", "Output mode: origin|custom")
	fs.StringVar(&cfg.Mode, "m", "origin", "Shorthand for --mode")
	fs.BoolVar(&cfg.Clean, "clean", false, "Clean output directory before processing")
	fs.BoolVar(&cfg.NoDbPath, "no-db-path", false, "Don't include database name in output path")
	fs.StringVar(&cfg.BlacklistDb, "blacklist-db", "^(template|postgres)", "Exclude databases matching pattern")
	fs.StringVar(&cfg.WhitelistDb, "whitelist-db", "", "Only include databases matching pattern")
	fs.StringVar(&cfg.ExcludeObj, "exclude-obj", "", "Exclude object types matching pattern")
	fs.BoolVar(&cfg.AclFiles, "acl-files", false, "Save ACLs to separate .acl.sql files")
	fs.BoolVar(&cfg.MoveRoles, "move-roles", false, "Move role files under database directory")
	fs.BoolVar(&cfg.Quiet, "quiet", false, "Suppress informational output")
	fs.BoolVar(&cfg.Version, "version", false, "Print version and exit")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Print what would be extracted without writing")

	usage := func(w io.Writer) {
		_, _ = fmt.Fprintf(w, "pg_atropos v%s - Split PostgreSQL custom-format dumps into organized files\n\n", version)
		_, _ = fmt.Fprintf(w, "Usage:\n  pg_atropos [options]\n\n")
		_, _ = fmt.Fprintf(w, "Options:\n")
		fs.SetOutput(w)
		fs.PrintDefaults()
	}
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage(os.Stdout)
			exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		usage(os.Stderr)
		exit(2)
	}

	if cfg.Version {
		fmt.Printf("pg_atropos v%s\n", version)
		exit(0)
	}

	if cfg.Mode != "origin" && cfg.Mode != "custom" {
		fmt.Fprintf(os.Stderr, "Error: mode must be 'origin' or 'custom'\n")
		exit(1)
	}

	for name, pattern := range map[string]string{
		"blacklist-db": cfg.BlacklistDb,
		"whitelist-db": cfg.WhitelistDb,
		"exclude-obj":  cfg.ExcludeObj,
	} {
		if _, err := regexp.Compile(pattern); err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid --%s pattern: %v\n", name, err)
			exit(1)
		}
	}

	return cfg
}

// ---------- DB name detection ----------

var reDbname = regexp.MustCompile(`^;.*dbname:\s*(.*)`)

func detectDbname(dumpFile string) string {
	cmd := exec.Command("pg_restore", "--list", dumpFile)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if m := reDbname.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

// ---------- Database filtering ----------

func checkDbFilter(dbname, blacklist, whitelist string) bool {
	if dbname == "" {
		return true
	}
	if whitelist != "" {
		matched, _ := regexp.MatchString(whitelist, dbname)
		if !matched {
			return false
		}
	}
	if blacklist != "" {
		matched, _ := regexp.MatchString(blacklist, dbname)
		if matched {
			return false
		}
	}
	return true
}

// ---------- Header parsing ----------

var reHeader = regexp.MustCompile(`^-- (?:Data for )?Name: (.*?); Type: (.*?); Schema: (.*?); Owner: `)

func parseHeader(line string) (name, objType, schema string) {
	m := reHeader.FindStringSubmatch(line)
	if m == nil {
		return "", "", ""
	}
	name = strings.Trim(strings.Trim(m[1], `"`), " ")
	objType = strings.Trim(m[2], " ")
	schema = strings.Trim(strings.Trim(m[3], `"`), " ")
	return
}

// ---------- Object writer ----------

var reCache = make(map[string]*regexp.Regexp)

// compileCached compiles a pattern once; invalid patterns yield nil.
func compileCached(pattern string) *regexp.Regexp {
	re, ok := reCache[pattern]
	if !ok {
		re, _ = regexp.Compile(pattern)
		reCache[pattern] = re
	}
	return re
}

// safeName makes an object name usable as a single path component, so that
// names such as the "/" operator or "a/b" cannot escape their directory.
func safeName(s string) string {
	bad := `/\`
	if runtime.GOOS == "windows" {
		bad += `:*?"<>|`
	}
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(bad, r) {
			fmt.Fprintf(&b, "%%%02X", r)
		} else {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "." || out == ".." {
		out = strings.ReplaceAll(out, ".", "%2E")
	}
	return out
}

var dirsCreated = make(map[string]bool)

func mkdirCached(dir string) {
	if !dirsCreated[dir] {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory %s: %v\n", dir, err)
			exit(1)
		}
		dirsCreated[dir] = true
	}
}

func writeObject(cfg *Config, dbname, curName, curType, curSchema, content string, total *int) {
	if curType == "" || content == "" {
		return
	}

	// Exclude filter
	if cfg.ExcludeObj != "" {
		if re := compileCached(cfg.ExcludeObj); re != nil && re.MatchString(curType) {
			return
		}
	}

	// TABLE DATA is skipped
	if curType == "TABLE DATA" {
		return
	}

	// Strip trailing newline from content accumulation
	content = strings.TrimSuffix(content, "\n")

	// Extract ACL/COMMENT object qualifier ("TABLE users" -> obj="users")
	objRest := curName
	if curType == "COMMENT" || curType == "ACL" {
		if parts := strings.SplitN(curName, " ", 2); len(parts) == 2 {
			objRest = parts[1]
		}
	}

	// Build db path component
	dbpart := ""
	if dbname != "" && !cfg.NoDbPath {
		dbpart = safeName(dbname) + "/"
	}

	// SCHEMA special case
	if curType == "SCHEMA" {
		dir := filepath.Join(cfg.OutputDir, dbpart+safeName(curName))
		fpath := filepath.Join(dir, safeName(curName)+".sql")
		if !cfg.DryRun {
			mkdirCached(dir)
			appendToFile(fpath, content)
		}
		*total++
		if cfg.DryRun {
			fmt.Fprintf(os.Stderr, "  %s\n", fpath)
		}
		return
	}

	// Determine type dir and object filename
	typeDir := curType
	objName := curName

	if cfg.Mode == "custom" {
		// Custom mode: lowercase dirs
		switch curType {
		case "SEQUENCE OWNED BY", "SEQUENCE SET":
			typeDir = "sequence"
		case "FUNCTION", "PROCEDURE":
			typeDir = "function"
			objName = funcFilename(curName)
		case "COMMENT", "ACL":
			typeDir = "acl"
			objName = objRest
			if strings.Contains(objRest, "(") {
				objName = funcFilename(objRest)
			}
		default:
			typeDir = strings.ToLower(curType)
		}
	} else {
		// Origin mode
		switch curType {
		case "FUNCTION", "PROCEDURE":
			objName = funcFilename(curName)
		case "COMMENT", "ACL":
			objName = objRest
			if strings.Contains(objRest, "(") {
				objName = funcFilename(objRest)
			}
		}
	}

	if objName == "" {
		return
	}

	// Schema dir: schema "-" objects with --move-roles skip the schema dir
	objName = safeName(objName)
	schemaDir := safeName(curSchema)
	if cfg.MoveRoles && schemaDir == "-" {
		schemaDir = ""
	}

	// Build final path
	relPath := filepath.Join(dbpart, schemaDir, typeDir, objName+".sql")
	fullPath := filepath.Join(cfg.OutputDir, relPath)

	if cfg.AclFiles && curType == "ACL" {
		fullPath = strings.TrimSuffix(fullPath, ".sql") + ".acl.sql"
	}

	if !cfg.DryRun {
		mkdirCached(filepath.Dir(fullPath))
		appendToFile(fullPath, content)
	}
	*total++
	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "  %s\n", fullPath)
	}
}

// filesWritten tracks files written during this run: the first write
// truncates leftovers from a previous run, later writes append.
var filesWritten = make(map[string]bool)

func appendToFile(path, content string) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	appending := filesWritten[path]
	if appending {
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0640)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file %s: %v\n", path, err)
		exit(1)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", path, err)
			exit(1)
		}
	}()
	filesWritten[path] = true
	if appending {
		if _, err := f.WriteString("\n"); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", path, err)
			exit(1)
		}
	}
	if _, err := f.WriteString(content); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", path, err)
		exit(1)
	}
	if _, err := f.WriteString("\n"); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing to %s: %v\n", path, err)
		exit(1)
	}
}

// ---------- Function name helpers ----------

func funcArgsHash(args string) string {
	if args == "" {
		return "000000"
	}
	h := md5.Sum([]byte(args))
	return hex.EncodeToString(h[:])[:6]
}

func funcFilename(fname string) string {
	// Extract args: functionName(arg1, arg2) -> arg1, arg2
	parenIdx := strings.Index(fname, "(")
	if parenIdx < 0 {
		return fname
	}
	endIdx := strings.LastIndex(fname, ")")
	if endIdx < parenIdx {
		return fname
	}
	baseName := fname[:parenIdx]
	argsPart := fname[parenIdx+1 : endIdx] // strip parens
	hash := funcArgsHash(argsPart)
	return baseName + "-" + hash
}

// ---------- Parser ----------

// cleanContent strips the comment frame ("--") that precedes the next
// header, plus surrounding blank lines.
func cleanContent(s string) string {
	s = strings.TrimRight(s, "\n\t ")
	if s == "--" {
		return ""
	}
	s = strings.TrimSuffix(s, "\n--")
	return strings.TrimRight(s, "\n\t ")
}

func parseAndWrite(rc io.Reader, cfg *Config, dbname string) int {
	// bufio.Reader (not Scanner): COPY data lines can be arbitrarily long.
	reader := bufio.NewReaderSize(rc, 1024*1024)

	var curName, curType, curSchema string
	var content strings.Builder
	total := 0
	inHeader := true
	skipObj := false
	afterHeader := false
	prevLine := ""

	flush := func() {
		if curType != "" && !skipObj {
			writeObject(cfg, dbname, curName, curType, curSchema, cleanContent(content.String()), &total)
		}
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Error reading SQL stream: %v\n", err)
			exit(1)
		}
		if err == io.EOF && line == "" {
			break
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		prev := prevLine
		prevLine = line

		// Check for header (always framed by a preceding "--" line)
		if prev == "--" {
			if name, typ, schema := parseHeader(line); name != "" {
				flush()
				curName = name
				curType = typ
				curSchema = schema
				content.Reset()
				inHeader = false
				afterHeader = true
				skipObj = (curType == "TABLE DATA")
				continue
			}
		}

		if inHeader {
			continue
		}

		// Closing "--" of the header comment frame
		if afterHeader {
			afterHeader = false
			if line == "--" {
				continue
			}
		}

		// Check for end marker
		if strings.HasPrefix(line, "-- PostgreSQL database dump complete") {
			flush()
			curType = ""
			skipObj = false
			continue
		}

		// Accumulate content
		if !skipObj {
			// Skip leading blank lines
			if content.Len() == 0 && strings.TrimSpace(line) == "" {
				continue
			}
			if content.Len() > 0 {
				content.WriteString("\n")
			}
			content.WriteString(line)
		}
	}

	// Flush last object
	flush()

	return total
}
