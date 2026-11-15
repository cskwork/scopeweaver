"use client";

import * as React from "react";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  type ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import { api } from "@/lib/api";
import type { FindingTrendPoint } from "@/lib/types";
import { translate as swt } from "@/i18n/runtime";

// Severity series, ordered most→least urgent. Colours match the dashboard
// severity chips so the two read as one system. "all" is the combined total,
// which the API already returns as `total`; its label reuses the same
// interface.m0154-m0157 keys as the severity chips, plus m2889 for "Combined".
const SERIES = [
  { key: "all", dataKey: "total", labelKey: "interface.m2889", color: "hsl(220 90% 60%)" },
  { key: "critical", dataKey: "critical", labelKey: "interface.m0154", color: "hsl(350 89% 60%)" },
  { key: "high", dataKey: "high", labelKey: "interface.m0155", color: "hsl(0 84% 60%)" },
  { key: "medium", dataKey: "medium", labelKey: "interface.m0156", color: "hsl(38 92% 50%)" },
  { key: "low", dataKey: "low", labelKey: "interface.m0157", color: "hsl(215 20% 65%)" },
] as const;

type SeriesKey = (typeof SERIES)[number]["key"];
type Bucket = "hour" | "day" | "week";

// Keyed by dataKey so the tooltip can resolve a label for every line. Labels
// are getters so they follow the active locale without rebuilding the module.
const chartConfig: ChartConfig = {
  total: { get label() { return swt("interface.m2889"); }, color: "hsl(220 90% 60%)" },
  critical: { get label() { return swt("interface.m0154"); }, color: "hsl(350 89% 60%)" },
  high: { get label() { return swt("interface.m0155"); }, color: "hsl(0 84% 60%)" },
  medium: { get label() { return swt("interface.m0156"); }, color: "hsl(38 92% 50%)" },
  low: { get label() { return swt("interface.m0157"); }, color: "hsl(215 20% 65%)" },
};

const RANGES: { key: string; label: string; days: number; bucket: Bucket }[] = [
  { key: "24h", label: "24h", days: 1, bucket: "hour" },
  { key: "7d", label: "7d", days: 7, bucket: "day" },
  { key: "30d", label: "30d", days: 30, bucket: "day" },
  { key: "all", label: "All", days: 0, bucket: "day" },
];

// Daily = findings discovered in each bucket. Cumulative = running total up to
// that bucket. Cumulative always accumulates over the whole history so the line
// ends at the true total; the range control only zooms the visible tail.
const MODES = [
  { key: "daily", labelKey: "interface.m2890" },
  { key: "cumulative", labelKey: "interface.m2891" },
] as const;

type Mode = (typeof MODES)[number]["key"];

export interface FindingTrendChartProps {
  // Task filter options; the dashboard already fetches these via findingStats().
  tasks: { id: string | number; name?: string; description: string; count: number }[];
}

/**
 * Findings over time, selectable by task, severity, or both.
 *
 * The series is aggregated server-side (GET /api/exploration/findings/trend) so
 * the payload is O(buckets) rather than O(findings) — the dashboard polls this
 * page every 5s and the raw findings array is capped at 500 rows.
 */
export function FindingTrendChart({ tasks }: FindingTrendChartProps) {
  const [range, setRange] = React.useState<string>("7d");
  const [mode, setMode] = React.useState<Mode>("daily");
  const [task, setTask] = React.useState<string>("all");
  const [enabled, setEnabled] = React.useState<Record<SeriesKey, boolean>>({
    all: true,
    critical: true,
    high: true,
    medium: true,
    low: true,
  });
  const [points, setPoints] = React.useState<FindingTrendPoint[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [failed, setFailed] = React.useState(false);

  const cfg = RANGES.find((r) => r.key === range) ?? RANGES[1];

  // Cumulative mode must accumulate from the very first finding, so it always
  // requests the full history. Daily mode honours the selected range.
  const fetchDays = mode === "cumulative" ? 0 : cfg.days;

  React.useEffect(() => {
    let alive = true;
    setLoading(true);
    api
      .findingTrend({ task, bucket: cfg.bucket, days: fetchDays })
      .then((res) => {
        if (!alive) return;
        setPoints(res?.points ?? []);
        setFailed(false);
      })
      .catch(() => {
        if (!alive) return;
        setPoints([]);
        setFailed(true);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [task, cfg.bucket, fetchDays]);

  // Recharts needs a category axis to be continuous, so fill the gaps the server
  // omits — otherwise a quiet day would be silently skipped and the line would
  // imply activity that did not happen.
  const data = React.useMemo(() => {
    if (points.length === 0) return [];
    const stepMs = cfg.bucket === "hour" ? 3_600_000 : cfg.bucket === "week" ? 604_800_000 : 86_400_000;
    const byBucket = new Map(points.map((p) => [Date.parse(p.bucket), p]));
    const start = Math.min(...points.map((p) => Date.parse(p.bucket)));
    const end = Math.max(...points.map((p) => Date.parse(p.bucket)));

    // Fill every bucket in [start, end] so the series is continuous.
    const filled: { t: number; p: FindingTrendPoint | undefined }[] = [];
    for (let t = start; t <= end; t += stepMs) filled.push({ t, p: byBucket.get(t) });

    if (mode === "cumulative") {
      // Accumulate over the FULL history so the final point equals the true
      // total; then trim to the selected range for display only.
      const cutoff = cfg.days > 0 ? end - cfg.days * 86_400_000 : Number.NEGATIVE_INFINITY;
      let total = 0, critical = 0, high = 0, medium = 0, low = 0;
      const out: (FindingTrendPoint & { label: string })[] = [];
      for (const { t, p } of filled) {
        total += p?.total ?? 0;
        critical += p?.critical ?? 0;
        high += p?.high ?? 0;
        medium += p?.medium ?? 0;
        low += p?.low ?? 0;
        if (t < cutoff) continue;
        const d = new Date(t);
        out.push({
          bucket: p?.bucket ?? d.toISOString(),
          total, critical, high, medium, low,
          label:
            cfg.bucket === "hour"
              ? d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
              : d.toLocaleDateString(undefined, { month: "short", day: "numeric" }),
        });
      }
      return out;
    }

    return filled.map(({ t, p }) => {
      const d = new Date(t);
      return {
        bucket: p?.bucket ?? d.toISOString(),
        total: p?.total ?? 0,
        critical: p?.critical ?? 0,
        high: p?.high ?? 0,
        medium: p?.medium ?? 0,
        low: p?.low ?? 0,
        label:
          cfg.bucket === "hour"
            ? d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
            : d.toLocaleDateString(undefined, { month: "short", day: "numeric" }),
      };
    });
  }, [points, cfg.bucket, cfg.days, mode]);

  const activeSeries = SERIES.filter((s) => enabled[s.key]);
  const anySeries = activeSeries.length > 0;

  return (
    <Card className="overflow-hidden">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 pb-2">
        <CardTitle className="text-sm">Findings over time</CardTitle>

        <div className="flex flex-wrap items-center gap-1.5">
          {/* Task filter. */}
          <select
            value={task}
            onChange={(e) => setTask(e.target.value)}
            className="h-6 max-w-[190px] rounded-md border border-border bg-background px-1.5 text-[10px] text-foreground"
            aria-label={swt("interface.m2892")}
          >
            <option value="all">All tasks</option>
            {tasks.map((t) => (
              <option key={String(t.id)} value={String(t.id)}>
                {`#${t.id} ${t.name || t.description || "unnamed"}`.slice(0, 46)}
              </option>
            ))}
          </select>

          {/* Daily vs cumulative. */}
          <div className="flex items-center gap-0.5 rounded-md border border-border p-0.5">
            {MODES.map((m) => (
              <button
                key={m.key}
                type="button"
                onClick={() => setMode(m.key)}
                className={
                  "rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors " +
                  (mode === m.key
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:text-foreground")
                }
                aria-pressed={mode === m.key}
              >
                {swt(m.labelKey)}
              </button>
            ))}
          </div>

          {/* Time range. */}
          <div className="flex items-center gap-0.5 rounded-md border border-border p-0.5">
            {RANGES.map((r) => (
              <button
                key={r.key}
                type="button"
                onClick={() => setRange(r.key)}
                className={
                  "rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors " +
                  (range === r.key
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:text-foreground")
                }
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>
      </CardHeader>

      {/* Severity toggles — click to isolate a series. */}
      <CardContent className="pb-2">
        <div className="mb-2 flex flex-wrap items-center gap-1.5">
          {SERIES.map((s) => (
            <button
              key={s.key}
              type="button"
              onClick={() => setEnabled((p) => ({ ...p, [s.key]: !p[s.key] }))}
              className={
                "flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium transition-opacity " +
                (enabled[s.key] ? "opacity-100" : "opacity-40")
              }
              aria-pressed={enabled[s.key]}
            >
              <span className="size-1.5 rounded-full" style={{ background: s.color }} />
              {swt(s.labelKey)}
            </button>
          ))}
        </div>

        {failed ? (
          <div className="flex h-[180px] items-center justify-center text-[11px] text-muted-foreground">
            Could not load findings trend.
          </div>
        ) : !anySeries ? (
          <div className="flex h-[180px] items-center justify-center text-[11px] text-muted-foreground">
            Select at least one severity.
          </div>
        ) : data.length === 0 ? (
          <div className="flex h-[180px] items-center justify-center text-[11px] text-muted-foreground">
            {loading ? swt("interface.m0260") : swt("interface.m2893")}
          </div>
        ) : (
          <ChartContainer config={chartConfig} className="h-[180px] w-full">
            <LineChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} className="stroke-border" />
              <XAxis dataKey="label" tickLine={false} axisLine={false} fontSize={9} interval="preserveStartEnd" minTickGap={24} />
              <YAxis tickLine={false} axisLine={false} fontSize={9} allowDecimals={false} width={40} />
              <ChartTooltip content={<ChartTooltipContent />} />
              {activeSeries.map((s) => (
                <Line
                  key={s.key}
                  type="monotone"
                  dataKey={s.dataKey}
                  name={swt(s.labelKey)}
                  stroke={s.color}
                  strokeWidth={s.key === "all" ? 2 : 1.5}
                  dot={false}
                  activeDot={{ r: 2.5 }}
                  isAnimationActive={false}
                />
              ))}
            </LineChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  );
}