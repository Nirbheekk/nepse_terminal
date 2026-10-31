import { api, ApiError } from "@/lib/api";
import { Notice, Panel, Provenance, UnavailableState } from "@/components/ui";
import { Heatmap } from "@/components/charts/heatmap";
import { PerformanceAnalysis } from "@/components/charts/performance-analysis";
import type { Company, MarketBreadth, SectorComparisonItem } from "@/lib/schemas";

export const dynamic = "force-dynamic";

/**
 * Sectors: breadth, sector comparison and the constituent heat map.
 *
 * Each of the three datasets is fetched independently and its failure is
 * reported. Swallowing an error here previously left the heat map empty with no
 * explanation, which reads as "every stock is flat".
 */
export default async function SectorsPage() {
  let companies: Company[] = [];
  let breadth: MarketBreadth | null = null;
  let sectorComparison: SectorComparisonItem[] = [];
  const problems: string[] = [];

  try {
    // 200 is the per-page cap on /companies; asking for more returns 400.
    companies = (await api.companies({ limit: 200 })).data;
  } catch (err) {
    problems.push(`Constituents: ${err instanceof ApiError ? err.message : "unavailable"}`);
  }

  try {
    breadth = (await api.marketBreadth()).data;
  } catch (err) {
    problems.push(`Breadth: ${err instanceof ApiError ? err.message : "unavailable"}`);
  }

  try {
    sectorComparison = (await api.sectorComparison("1d")).data;
  } catch (err) {
    problems.push(`Sector comparison: ${err instanceof ApiError ? err.message : "unavailable"}`);
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-slate-100">Sectors</h1>
        <p className="mt-1 text-xs text-surface-muted">
          Sector performance and a heat map of constituents. Colour intensity is the percentage change
          against the previous close in the stored session.
        </p>
      </div>

      {problems.length > 0 ? (
        <Notice tone="bad" title="Part of this view could not be loaded" detail={problems.join(" · ")} />
      ) : null}

      <Panel title="Market breadth & distribution">
        <div className="p-4">
          <PerformanceAnalysis initialBreadth={breadth} initialSectors={sectorComparison} />
        </div>
      </Panel>

      <Panel title="Constituent heat map">
        {companies.length === 0 ? (
          <UnavailableState
            title="No constituents to draw"
            detail="The company list could not be loaded, so there is nothing to shade. This is not a flat market — it is a missing input, and it is reported above rather than drawn as zeroes."
          />
        ) : (
          <div className="p-4">
            <Heatmap companies={companies} />
          </div>
        )}
      </Panel>

      <Provenance source="postgres:daily_ohlcv + postgres:companies" license="public_unverified" />
    </div>
  );
}
