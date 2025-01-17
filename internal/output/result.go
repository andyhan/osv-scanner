package output

import (
	"encoding/json"

	"github.com/google/osv-scanner/pkg/models"
	"golang.org/x/exp/slices"
)

type pkgWithSource struct {
	Package models.PackageInfo
	Source  models.SourceInfo
}

// Custom implementation of this unique set map to allow it to serialize to JSON
type pkgSourceSet map[pkgWithSource]struct{}

func (pss *pkgSourceSet) MarshalJSON() ([]byte, error) {
	res := []pkgWithSource{}

	for v := range *pss {
		res = append(res, v)
	}

	return json.Marshal(res)
}

func (pss *pkgSourceSet) UnmarshalJSON(data []byte) error {
	aux := []pkgWithSource{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*pss = make(pkgSourceSet)
	for _, pws := range aux {
		(*pss)[pws] = struct{}{}
	}

	return nil
}

// groupFixedVersions builds the fixed versions for each ID Group, with keys formatted like so:
// `Source:ID`
func groupFixedVersions(flattened []models.VulnerabilityFlattened) map[string][]string {
	groupFixedVersions := map[string][]string{}

	// Get the fixed versions indexed by each group of vulnerabilities
	// Prepend source path as same vulnerability in two projects should be counted twice
	// Remember to sort and compact before displaying later
	for _, vf := range flattened {
		groupIdx := vf.Source.String() + ":" + vf.GroupInfo.IndexString()
		pkg := models.Package{
			Ecosystem: models.Ecosystem(vf.Package.Ecosystem),
			Name:      vf.Package.Name,
		}
		groupFixedVersions[groupIdx] =
			append(groupFixedVersions[groupIdx], vf.Vulnerability.FixedVersions()[pkg]...)
	}

	// Remove duplicates
	for k := range groupFixedVersions {
		fixedVersions := groupFixedVersions[k]
		slices.Sort(fixedVersions)
		groupFixedVersions[k] = slices.Compact(fixedVersions)
	}

	return groupFixedVersions
}

// groupedSARIFFinding groups vulnerabilities by aliases
type groupedSARIFFinding struct {
	DisplayID string
	PkgSource pkgSourceSet
	// AliasedVulns contains vulns that are OSV vulnerabilities
	AliasedVulns map[string]models.Vulnerability
	// AliasedIDList contains all aliased IDs, including ones that are not OSV (e.g. CVE IDs)
	// Sorted by idSortFunc, therefore the first element will be the display ID
	AliasedIDList []string
}

// mapIDsToGroupedSARIFFinding creates a map over all vulnerability IDs, with aliased vuln IDs
// pointing to the same groupedSARIFFinding object
func mapIDsToGroupedSARIFFinding(vulns *models.VulnerabilityResults) map[string]*groupedSARIFFinding {
	// Map of vuln IDs to their respective groupedSARIFFinding
	results := map[string]*groupedSARIFFinding{}

	for _, res := range vulns.Results {
		for _, pkg := range res.Packages {
			for _, gi := range pkg.Groups {
				var data *groupedSARIFFinding
				// See if this vulnerability group already exists (from another package or source)
				for _, id := range gi.IDs {
					existingData, ok := results[id]
					if ok {
						data = existingData
						break
					}
				}
				// If not create this group
				if data == nil {
					data = &groupedSARIFFinding{
						PkgSource:    make(pkgSourceSet),
						AliasedVulns: make(map[string]models.Vulnerability),
					}
				}
				// Point all the IDs of the same group to the same data, either newly created or existing
				for _, id := range gi.IDs {
					results[id] = data
				}
			}
			for _, v := range pkg.Vulnerabilities {
				newPkgSource := pkgWithSource{
					Package: pkg.Package,
					Source:  res.Source,
				}
				entry := results[v.ID]
				entry.PkgSource[newPkgSource] = struct{}{}
				entry.AliasedVulns[v.ID] = v
				entry.AliasedIDList = append(entry.AliasedIDList, v.ID)
				entry.AliasedIDList = append(entry.AliasedIDList, v.Aliases...)
			}
		}
	}

	for _, gs := range results {
		slices.SortFunc(gs.AliasedIDList, idSortFunc)
		gs.AliasedIDList = slices.Compact(gs.AliasedIDList)
		gs.DisplayID = gs.AliasedIDList[0]
	}

	return results
}
