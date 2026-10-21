package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"euromni/internal/artifact"
	"euromni/internal/dbdialect"
	"euromni/internal/pipeline"
	"euromni/internal/seed"
	"euromni/internal/store"
	"euromni/internal/wasm"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("prestashop-import", flag.ContinueOnError)
	archivePath := flags.String("archive", "", "path to the upstream PrestaShop module ZIP")
	platformVersion := flags.String("platform-version", "", "PrestaShop platform version used for this pilot")
	shopID := flags.String("shop-id", seed.ShopID, "demo shop UUID (the isolated importer only permits the seeded demo shop)")
	days := flags.Int("days", 30, "new-product window in days")
	limit := flags.Int("limit", 8, "maximum products returned")
	install := flags.Bool("install", false, "publish the generated Wasm into the isolated development CMS")
	publishOnly := flags.Bool("publish-only", false, "publish the generated Wasm without changing any shop installation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *install && *publishOnly {
		return fmt.Errorf("--install and --publish-only are mutually exclusive")
	}
	if strings.TrimSpace(*archivePath) == "" || strings.TrimSpace(*platformVersion) == "" {
		return fmt.Errorf("--archive and --platform-version are required")
	}
	if *shopID != seed.ShopID {
		return fmt.Errorf("only the seeded demo shop is allowed")
	}
	settings := store.NewProductsSettings{Days: *days, Limit: *limit}
	if err := store.ValidateNewProductsQuery(settings.Days, settings.Limit); err != nil {
		return err
	}
	archiveInfo, err := os.Lstat(*archivePath)
	if err != nil {
		return fmt.Errorf("inspect module archive: %w", err)
	}
	if !archiveInfo.Mode().IsRegular() || archiveInfo.Size() > pipeline.MaxPrestaShopArchiveBytes {
		return fmt.Errorf("module archive must be a regular file no larger than %d bytes", pipeline.MaxPrestaShopArchiveBytes)
	}
	archiveBytes, err := os.ReadFile(filepath.Clean(*archivePath))
	if err != nil {
		return fmt.Errorf("read module archive: %w", err)
	}
	pkg, err := pipeline.ReadPrestaShopNewProductsArchive(archiveBytes)
	if err != nil {
		return err
	}
	ir, wasmBytes, err := pipeline.BuildPrestaShopNewProductsArchive(archiveBytes, *platformVersion)
	if err != nil {
		return err
	}
	manifest, err := pipeline.NewBuildManifest(ir, wasmBytes)
	if err != nil {
		return err
	}
	fmt.Printf("validated module=%s version=%s archive_sha256=%s wasm_sha256=%s\n", pkg.Name, pkg.Version, pkg.ArchiveSHA, manifest.WASMSHA256)
	if !*install && !*publishOnly {
		fmt.Println("dry run only; nothing was published")
		return nil
	}
	if strings.TrimSpace(os.Getenv("EUROMNI_ENVIRONMENT")) != "development" && *install {
		return fmt.Errorf("--install requires EUROMNI_ENVIRONMENT=development")
	}
	if strings.TrimSpace(os.Getenv("EUROMNI_ENVIRONMENT")) != "development" && *publishOnly {
		return fmt.Errorf("--publish-only requires EUROMNI_ENVIRONMENT=development")
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if err := validateTestDatabaseURL(dsn); err != nil {
		return err
	}
	artifactRoot := strings.TrimSpace(os.Getenv("EUROMNI_ARTIFACT_DIR"))
	if artifactRoot == "" {
		return fmt.Errorf("EUROMNI_ARTIFACT_DIR is required for publication")
	}
	database, err := store.Open(dsn)
	if err != nil {
		return fmt.Errorf("connect to test CMS database: %w", err)
	}
	defer database.SQL.Close()
	if database.Dialect != dbdialect.PostgreSQL {
		return fmt.Errorf("PrestaShop pilot installation requires PostgreSQL")
	}
	var shopSlug string
	if err := database.SQL.QueryRowContext(context.Background(), `SELECT slug FROM shops WHERE id::text = $1`, *shopID).Scan(&shopSlug); err != nil {
		return fmt.Errorf("verify demo shop: %w", err)
	}
	if shopSlug != "atelier-demo" {
		return fmt.Errorf("target shop is not the seeded demo shop")
	}
	artifactStore, err := artifact.NewFileStore(artifactRoot)
	if err != nil {
		return fmt.Errorf("open private module artifact store: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if *publishOnly {
		var moduleID string
		if err := database.SQL.QueryRowContext(ctx, `SELECT id::text FROM modules WHERE name = $1`, store.NewProductsModuleName).Scan(&moduleID); err != nil {
			return fmt.Errorf("find registered PrestaShop module: %w", err)
		}
		stored, builtManifest, err := pipeline.PublishModuleBuildOnly(ctx, artifactStore, database, moduleID, ir, wasmBytes)
		if err != nil {
			return err
		}
		storedWasm, err := artifactStore.Get(ctx, stored.SHA256)
		if err != nil {
			return fmt.Errorf("read published Wasm: %w", err)
		}
		if err := builtManifest.Validate(ir, storedWasm); err != nil {
			return fmt.Errorf("verify published build: %w", err)
		}
		fmt.Printf("published without promotion module_id=%s artifact_sha256=%s; shop installations unchanged\n", moduleID, stored.SHA256)
		return nil
	}
	stored, builtManifest, moduleID, err := pipeline.PublishNewProductsModuleBuild(ctx, artifactStore, database, *shopID, settings, ir, wasmBytes)
	if err != nil {
		return err
	}
	storedWasm, err := artifactStore.Get(ctx, stored.SHA256)
	if err != nil {
		return fmt.Errorf("read published Wasm: %w", err)
	}
	if err := builtManifest.Validate(ir, storedWasm); err != nil {
		return fmt.Errorf("verify published build: %w", err)
	}
	runtime := wasm.NewRuntime(&wasm.Host{Products: database, Categories: database})
	defer runtime.Close()
	if err := runtime.LoadInstalled(ctx, database, *shopID, moduleID, storedWasm); err != nil {
		return fmt.Errorf("load installed module: %w", err)
	}
	results, err := runtime.Run(ctx, database, *shopID, moduleID, "run")
	if err != nil {
		return fmt.Errorf("execute installed Wasm: %w", err)
	}
	if len(results) != 1 {
		return fmt.Errorf("installed module returned an unexpected result")
	}
	resultBytes, ok := results[0].(int32)
	if !ok || resultBytes <= 0 {
		return fmt.Errorf("installed module returned an invalid ABI result")
	}
	fmt.Printf("installed shop=%s module_id=%s artifact_sha256=%s settings_days=%d settings_limit=%d runtime_result_bytes=%d\n",
		*shopID, moduleID, stored.SHA256, settings.Days, settings.Limit, resultBytes)
	return nil
}

func validateTestDatabaseURL(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required for installation")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return fmt.Errorf("installation requires a local PostgreSQL DATABASE_URL")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host != "postgres" && host != "localhost" && net.ParseIP(host) == nil {
		return fmt.Errorf("installation database host must be local")
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return fmt.Errorf("installation database IP must be loopback")
	}
	if strings.TrimPrefix(parsed.Path, "/") != "euromni" {
		return fmt.Errorf("installation database must be named euromni")
	}
	return nil
}
