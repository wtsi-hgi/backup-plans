import { MainProgrammes } from "./consts.js";
import { div, p, h2, canvas, br, span, source, i } from "./lib/html.js";
import { formatBytes } from "./lib/utils.js";
import type { SizeCount, BarChartRow } from "./types.js";
// import type { ChartConfiguration, ChartData } from "./chart.esm.js";
// @ts-ignore
import { Chart } from "./chart-wrapper.js";

// Chart.register(...registerables);

// const MainProgrammes = ["All", "Unknown"];
const colourClasses = ["bar-unplanned", "bar-nobackup", "bar-backup"];
const base = div();

function prepareData(programmeCounts: Map<string, Map<number, SizeCount>>) {
    const barChartData: BarChartRow[] = [];

    for (const programme of MainProgrammes) {
        const data = programmeCounts.get(programme)!

        const sizes = [-1, 0, 1, 2].map(i => data.get(i)?.size || 0n);
        const totalSize = sizes.reduce((total, size) => total + size, 0n);

        const sizeFractions = Array.from(data.entries())
            .sort(([keyA], [keyB]) => keyA - keyB)
            .map(([_, item]) => Number(100n * item.size / totalSize));
        const programmeFractions = largestRemainderRound(sizeFractions);

        barChartData.push({ Programme: programme, Fractions: programmeFractions, Sizes: sizes })
    }

    addManualToBackup(barChartData);

    return barChartData
}

function largestRemainderRound(values: number[]): number[] {
    const rounded = values.map(val => Math.floor(val));
    const diff = 100 - rounded.reduce((total, val) => total + val, 0);

    const indexed = values.map((value, index) => ({ value, index }));
    const sorted = [...indexed].sort((a, b) => (Math.floor(a.value) - a.value) - (Math.floor(b.value) - b.value));

    sorted.slice(0, diff).forEach(({ index }) => {
        rounded[index] += 1;
    })

    while (rounded.length < 4) {
        rounded.push(0);
    }

    return rounded
}

function addManualToBackup(data: BarChartRow[]) {
    for (const row of data) {
        row.Fractions[2] += row.Fractions[3]
        row.Sizes[2] += row.Sizes[3]
    }
}

function graphKey() {
    return div({ "id": "graphKey" }, [
        div("Unplanned"),
        div("No Backup"),
        div("Backup")
    ])
}

function generateBarChart(programmeCounts: Map<string, Map<number, SizeCount>>) {
    const barChartData = prepareData(programmeCounts);

    return div({ "class": "graph-container" }, [
        h2("Data Fraction per Programme"),
        graphKey(), br(),
        div({ "class": "graph-grid" }, [
            div({ "class": "graphAxisY" }, [
                div(),
                MainProgrammes.map(programme => [div(programme)]),
                div()
            ]),
            div({ "class": "graph-wrapper" }, [
                div({ "class": "graphA" }, [
                    barChartData.map(row => [
                        div({ "class": "graphRow" }, [...[0, 1, 2].flatMap(i =>
                            div({
                                "style": "width:" + row.Fractions[i] + "%;",
                                "title": row.Fractions[i] + "%",
                                "class": colourClasses[i],
                                "mouseenter": function (this: HTMLElement) { this.firstElementChild!.textContent = formatBytes(row.Sizes[i]) },
                                "mouseleave": function (this: HTMLElement) { this.firstElementChild!.textContent = row.Fractions[i] + "%" }
                            }, [row.Fractions[i]! !== 0 ? p(row.Fractions[i] + "%") : p()]))])
                    ])
                ]),
            ]),
            div(),
            div({ "class": "graphAxisX" }, [
                span("0%"),
                span("20%"),
                span("40%"),
                span("60%"),
                span("80%"),
                span("100%")
            ]),
        ]), br(),
    ])
}

function prepareDataAbsScale(programmeCounts: Map<string, Map<number, SizeCount>>) {
    const colours = ["f08080", "fec89a", "76c893"]
    const data = {
        labels: MainProgrammes,
        datasets: ["Unplanned", "No backup", "Backup"].map((backupType, i) => ({
            label: backupType,
            data: MainProgrammes.map(programme => {
                return Number((programmeCounts.get(programme)!.get(i - 1))?.size ?? 0);
            }),
            backgroundColor: "#" + colours[i]
        }))
    };

    return data
}

function cssVar(name: string) {
    return getComputedStyle(document.documentElement)
        .getPropertyValue(name)
        .trim();
}

function generateGroupedBarChart(programmeCounts: Map<string, Map<number, SizeCount>>) {
    const data = prepareDataAbsScale(programmeCounts);

    const config = {
        type: 'bar',
        data: data!,
        options: {
            responsive: false,
            // maintainAspectRatio: false,
            indexAxis: 'y',
            plugins: {
                legend: {
                    display: true,
                    position: 'top',
                    align: 'center',
                    labels: {
                        font: {
                            size: 14
                        },
                        boxWidth: 14,
                        boxHeight: 14,
                        padding: 8,
                        color: cssVar('--graph-label-colour'),
                    },
                    maxHeight: Infinity,
                    maxWidth: Infinity
                }
            },
            scales: {
                y: {
                    display: true,
                    stacked: false,
                    ticks: {
                        color: cssVar('--graph-label-colour'),
                        font: {
                            size: 14
                        }
                    },
                    grid: {
                        color: cssVar('--graph-accent'),
                        borderColor: cssVar('--graph-accent'),
                        lineWidth: 0.3
                    }
                },
                x: {
                    display: true,
                    type: 'logarithmic',
                    ticks: {
                        color: cssVar('--graph-accent'),
                        font: {
                            size: 14
                        }
                    },
                    grid: {
                        color: cssVar('--graph-accent'),
                        borderColor: cssVar('--graph-accent'),
                        lineWidth: 0.3
                    }
                }
            }
        },
    };

    const cvs = canvas({ "id": "logChart", "width": "1200", "height": "400" });

    new Chart(cvs, config);

    return div({ "class": "log-chart-container" }, [
        h2("Absolute scale comparison"),
        // graphKey(),
        div(cvs)
    ]);
}

function createGraphPage(programmeCounts: Map<string, Map<number, SizeCount>>) {
    base.appendChild(generateBarChart(programmeCounts));
    base.appendChild(generateGroupedBarChart(programmeCounts));
}

export default Object.assign(base, {
    init: createGraphPage
})