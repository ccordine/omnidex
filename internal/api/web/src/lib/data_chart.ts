import * as d3 from "d3";
import type { ChartSpec } from "./data_api";

const palette = ["#67e8f9", "#818cf8", "#f472b6", "#fbbf24", "#34d399", "#fb7185", "#a78bfa", "#2dd4bf"];

export function renderD3Chart(container: HTMLElement, chart: ChartSpec | undefined): void {
  container.innerHTML = "";
  if (!chart?.series?.length) return;

  const width = Math.max(280, container.clientWidth || 420);
  const height = 240;
  const margin = { top: 28, right: 16, bottom: 36, left: 48 };

  const svg = d3
    .select(container)
    .append("svg")
    .attr("viewBox", `0 0 ${width} ${height}`)
    .attr("class", "w-full max-w-full overflow-visible");

  if (chart.title) {
    svg
      .append("text")
      .attr("x", margin.left)
      .attr("y", 18)
      .attr("fill", "#e4e4e7")
      .attr("font-size", 12)
      .text(chart.title);
  }

  const innerWidth = width - margin.left - margin.right;
  const innerHeight = height - margin.top - margin.bottom;
  const g = svg.append("g").attr("transform", `translate(${margin.left},${margin.top})`);
  const series = chart.series.slice(0, 24);

  if (chart.type === "pie") {
    const radius = Math.min(innerWidth, innerHeight) / 2;
    const pie = d3.pie<ChartSpec["series"][number]>().value((d) => d.value);
    const arc = d3.arc<d3.PieArcDatum<ChartSpec["series"][number]>>().innerRadius(radius * 0.45).outerRadius(radius);
    const pieG = g.append("g").attr("transform", `translate(${innerWidth / 2},${innerHeight / 2})`);
    pieG
      .selectAll("path")
      .data(pie(series))
      .join("path")
      .attr("fill", (_, i) => palette[i % palette.length] ?? "#67e8f9")
      .attr("d", arc);
    return;
  }

  const x = d3
    .scaleBand()
    .domain(series.map((point) => point.label))
    .range([0, innerWidth])
    .padding(0.18);
  const y = d3
    .scaleLinear()
    .domain([0, d3.max(series, (d) => d.value) ?? 1])
    .nice()
    .range([innerHeight, 0]);

  g.append("g")
    .attr("transform", `translate(0,${innerHeight})`)
    .call(d3.axisBottom(x).tickFormat((value) => truncate(String(value), 10)))
    .call((axis) => axis.selectAll("text").attr("fill", "#a1a1aa").attr("font-size", 10))
    .call((axis) => axis.selectAll("line,path").attr("stroke", "#3f3f46"));

  g.append("g")
    .call(d3.axisLeft(y).ticks(5))
    .call((axis) => axis.selectAll("text").attr("fill", "#a1a1aa").attr("font-size", 10))
    .call((axis) => axis.selectAll("line,path").attr("stroke", "#3f3f46"));

  if (chart.type === "line") {
    const line = d3
      .line<ChartSpec["series"][number]>()
      .x((d) => (x(d.label) ?? 0) + x.bandwidth() / 2)
      .y((d) => y(d.value));
    g.append("path")
      .datum(series)
      .attr("fill", "none")
      .attr("stroke", "#67e8f9")
      .attr("stroke-width", 2)
      .attr("d", line);
    g.selectAll("circle")
      .data(series)
      .join("circle")
      .attr("cx", (d) => (x(d.label) ?? 0) + x.bandwidth() / 2)
      .attr("cy", (d) => y(d.value))
      .attr("r", 3)
      .attr("fill", "#67e8f9");
    return;
  }

  g.selectAll("rect")
    .data(series)
    .join("rect")
    .attr("x", (d) => x(d.label) ?? 0)
    .attr("y", (d) => y(d.value))
    .attr("width", x.bandwidth())
    .attr("height", (d) => innerHeight - y(d.value))
    .attr("fill", (_, i) => palette[i % palette.length] ?? "#67e8f9")
    .attr("rx", 3);
}

function truncate(value: string, max: number): string {
  return value.length > max ? `${value.slice(0, max - 1)}…` : value;
}

export function mountDataCharts(root: ParentNode): void {
  root.querySelectorAll("[data-d3-chart]").forEach((node) => {
    const element = node as HTMLElement;
    const raw = element.dataset.chartSpec || "";
    if (!raw) return;
    try {
      const chart = JSON.parse(raw) as ChartSpec;
      renderD3Chart(element, chart);
    } catch {
      element.textContent = "Chart unavailable";
    }
  });
}
