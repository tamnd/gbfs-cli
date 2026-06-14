package gbfs

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes GBFS as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/gbfs-cli/gbfs"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// gbfs:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone gbfs binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the GBFS driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "gbfs",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "gbfs",
			Short:  "Query public bike share data via GBFS feeds.",
			Long: `Query public bike share data via GBFS feeds.

gbfs reads public GBFS (General Bikeshare Feed Specification) feeds over
plain HTTPS, shapes station and system data into clean records, and prints
output that pipes into the rest of your tools. No API key required.

Supported systems: bkn (Citi Bike NYC), divvy (Chicago), bay (Bay Wheels),
capital (Capital Bikeshare DC).`,
			Site: "github.com/tamnd/gbfs-cli",
			Repo: "https://github.com/tamnd/gbfs-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:    "stations",
		Group:   "read",
		List:    true,
		Summary: "List bike share stations with availability",
		URIType: "station",
	}, listStations)

	kit.Handle(app, kit.OpMeta{
		Name:    "info",
		Group:   "read",
		Single:  true,
		Summary: "Show system information (name, URL, timezone)",
		URIType: "system",
	}, getInfo)
}

// newClient builds the GBFS client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	gcfg := DefaultConfig()
	if cfg.UserAgent != "" {
		gcfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		gcfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		gcfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		gcfg.Timeout = cfg.Timeout
	}
	return NewClient(gcfg), nil
}

// --- inputs ---

type stationsInput struct {
	System   string  `kit:"flag" help:"system ID (bkn, divvy, bay, capital)"`
	MinBikes int     `kit:"flag" help:"minimum bikes available filter"`
	Limit    int     `kit:"flag,inherit" help:"max results"`
	Client   *Client `kit:"inject"`
}

type infoInput struct {
	System string  `kit:"flag" help:"system ID (bkn, divvy, bay, capital)"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listStations(ctx context.Context, in stationsInput, emit func(Station) error) error {
	system := in.System
	if system == "" {
		system = "bkn"
	}
	stations, err := in.Client.ListStations(ctx, system, in.MinBikes)
	if err != nil {
		return mapErr(err)
	}
	n := 0
	for _, s := range stations {
		if in.Limit > 0 && n >= in.Limit {
			break
		}
		if err := emit(s); err != nil {
			return err
		}
		n++
	}
	return nil
}

func getInfo(ctx context.Context, in infoInput, emit func(*SystemInfo) error) error {
	system := in.System
	if system == "" {
		system = "bkn"
	}
	info, err := in.Client.GetSystemInfo(ctx, system)
	if err != nil {
		return mapErr(err)
	}
	return emit(info)
}

// --- Resolver ---

// Classify turns any accepted input into the canonical (type, id).
// Accepts "bkn", "divvy", "bay", "capital" as system IDs,
// and "station-<id>" as station references.
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	knownSystems := map[string]bool{"bkn": true, "divvy": true, "bay": true, "capital": true}
	if knownSystems[input] {
		return "system", input, nil
	}
	if strings.HasPrefix(input, "station-") {
		return "station", strings.TrimPrefix(input, "station-"), nil
	}
	return "", "", errs.Usage("unrecognized gbfs reference: %q", input)
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "system":
		urls := DefaultConfig().SystemURLs
		if base, ok := urls[id]; ok {
			// derive the web URL from system_information
			switch id {
			case "bkn":
				return "https://www.citibikenyc.com", nil
			case "divvy":
				return "https://www.divvybikes.com", nil
			case "bay":
				return "https://www.baywheels.com", nil
			case "capital":
				return "https://www.capitalbikeshare.com", nil
			default:
				return base, nil
			}
		}
		return "", errs.Usage("gbfs: unknown system %q", id)
	case "station":
		return "https://" + Host + "/station/" + id, nil
	default:
		return "", errs.Usage("gbfs has no resource type %q", uriType)
	}
}

// --- helpers ---

func mapErr(err error) error {
	return err
}
