import { BackupType, MainProgrammes } from "./consts.js";
import { div, p, h2, canvas, br, span, input, label, fieldset, legend } from "./lib/html.js";
import { formatBytes } from "./lib/utils.js";
import type { SizeCount, BarChartRow } from "./types.js";
import "./chart.umd.min.js";

export const Chart = (window as any).Chart;

const colourClasses = ["bar-unplanned", "bar-nobackup", "bar-backup"];
const base = div();

function prepareData(programmeCounts: Map<string, Map<BackupType, SizeCount>>) {
    const barChartData: BarChartRow[] = [];

    for (const programme of MainProgrammes) {
        const data = programmeCounts.get(programme)!
        console.log(data);

        const sizes = BackupType.all.slice(1, 4).map(bt => {
            if (bt === BackupType.BackupIBackup) {
                return ((data.get(bt)?.size || 0n) + (data.get(BackupType.BackupManual)?.size || 0n))
            }
            return data.get(bt)?.size || 0n
        });

        const totalSize = sizes.reduce((total, size) => total + size, 0n);

        const sizeFractions = BackupType.all.slice(1, 4)
            .sort((keyA, keyB) => +keyA - +keyB)
            .map((bt) => {
                if (bt === BackupType.BackupIBackup) {
                    return Number(100n * ((data.get(bt)?.size ?? 0n) + (data.get(BackupType.BackupManual)?.size ?? 0n)) / totalSize)
                }
                return Number(100n * (data.get(bt)?.size ?? 0n) / totalSize)
            });

        const programmeFractions = largestRemainderRound(sizeFractions);
        console.log(sizes, sizeFractions, totalSize);

        barChartData.push({ Programme: programme, Fractions: programmeFractions, Sizes: sizes })
    }

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

function graphKey() {
    return div({ "id": "graphKey" }, [
        div("Unplanned"),
        div("No Backup"),
        div("Backup")
    ])
}

function updateScale(chart: any, isLog: boolean) {
    chart.options.scales.x.type = isLog ? 'logarithmic' : 'linear';
    chart.update();
};

function updateAllRow(chart: any, programmeCounts: Map<string, Map<BackupType, SizeCount>>, justMainProgrammes: boolean) {
    chart.data = prepareDataAbsScale(programmeCounts, justMainProgrammes);
    chart.update();
};

function generateBarChart(programmeCounts: Map<string, Map<BackupType, SizeCount>>) {
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

function getSize(programmeCounts: Map<string, Map<BackupType, SizeCount>>, index: BackupType, programme: string) {
    return programmeCounts.get(programme)!.get(index)?.size ?? 0n
}

function getProgrammeSize(programme: string, btypes: BackupType[], programmeCounts: Map<string, Map<BackupType, SizeCount>>) {
    return btypes.reduce((acc, bt) => acc + getSize(programmeCounts, bt, programme), 0n);
}

function prepareDataAbsScale(programmeCounts: Map<string, Map<BackupType, SizeCount>>, justMainProgrammes: boolean) {
    const colours = ["#f08080", "#fec89a", "#76c893"];
    const labelMap = new Map<string, string>([
        ["warn", "Unplanned"],
        ["nobackup", "No Backup"],
        ["backup", "Backup"]
    ]);

    const data = {
        labels: MainProgrammes,
        datasets: [[BackupType.BackupWarn], [BackupType.BackupNone], [BackupType.BackupIBackup, ...BackupType.manual]].map((backupTypes, i) => ({
            label: labelMap.get(backupTypes[0].toString()),
            data: MainProgrammes
                .map(programme => {
                    if (justMainProgrammes && programme === "All") {
                        programme = ""
                    };
                    const size = getProgrammeSize(programme, backupTypes, programmeCounts);
                    return Number(size);
                }),
            backgroundColor: colours[i],
            hoverBackgroundColor: `color-mix(in srgb, ${colours[i]} 80%, transparent)`
        }))
    };

    return data;
}

function cssVar(name: string) {
    return getComputedStyle(document.documentElement)
        .getPropertyValue(name)
        .trim();
}

function generateGroupedBarChart(programmeCounts: Map<string, Map<BackupType, SizeCount>>) {
    const data = prepareDataAbsScale(programmeCounts, false);

    const config = {
        type: 'bar',
        data: data!,
        options: {
            animation: false,
            responsive: false,
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
                },
                tooltip: {
                    callbacks: {
                        label: (ctx: any) => {
                            const value = ctx.raw;
                            const backupType = ctx.dataset.label;
                            return `${backupType}: ${formatBytes(BigInt(value))}`;
                        },
                    },
                },
            },
            scales: {
                y: {
                    display: true,
                    stacked: true,
                    ticks: {
                        color: cssVar('--graph-label-colour'),
                        font: {
                            size: 14
                        }
                    },
                    grid: { drawOnChartArea: false },
                },
                x: {
                    display: true,
                    type: 'linear',
                    stacked: true,
                    ticks: {
                        color: cssVar('--graph-label-colour'),
                        font: {
                            size: 14
                        },
                        maxTicksLimit: 12,
                        callback: function (value: any) {
                            return `${formatBytes(BigInt(value))}`;
                        }
                    },
                    grid: {
                        color: cssVar('--graph-accent'),
                        lineWidth: 0.8,
                    }
                }
            }
        },
    };

    const cvs = canvas({ "id": "logChart", "width": "1200", "height": "400" });

    const chart = new Chart(cvs, config);

    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');

    mediaQuery.addEventListener('change', () => {
        chart.options.scales.x.ticks.color = cssVar('--graph-label-colour');
        chart.options.scales.y.ticks.color = cssVar('--graph-label-colour');
        chart.options.plugins.legend.labels.color = cssVar('--graph-label-colour');
        chart.update();
    });

    return div({ "class": "graph-container" }, [
        div({ "class": "log-chart-container" }, [
            h2("Absolute scale comparison"),
            div(cvs),
        ]),
        div({ "class": "chartButtonSection" }, [
            // button({
            //     "class": "scaleToggle", "click": (e: MouseEvent) => {
            //         const btn = e.currentTarget as HTMLButtonElement;

            //         isLog = !isLog;
            //         chart.options.scales.x.type = isLog ? 'logarithmic' : 'linear';
            //         btn.textContent = isLog ? 'Show linear scale' : 'Show logarithmic scale';
            //         chart.update();
            //     }
            // }, "Show linear scale"),
            // button({
            //     "class": "scaleToggle", "click": (e: MouseEvent) => {
            //         const btn = e.currentTarget as HTMLButtonElement;

            //         justMainProgrammes = !justMainProgrammes;
            //         console.log("Calling with justMainProgrammes as", justMainProgrammes);
            //         chart.data = prepareDataAbsScale(programmeCounts, justMainProgrammes);
            //         btn.textContent = justMainProgrammes ? 'Show total for all programmes' : 'Show total for just main programmes';
            //         chart.update();
            //     }
            // }, 'Show total for just main programmes'),
            div({ "class": "graphFilters" }, [
                fieldset({}, [
                    legend("Scale"),
                    input({ "type": "radio", "name": "graphScale", "id": "linear", "checked": "checked", change: () => updateScale(chart, false) }),
                    label({ "for": "linear" }, "Linear scale"),
                    input({ "type": "radio", "name": "graphScale", "id": "logrithmic", change: () => updateScale(chart, true) }),
                    label({ "for": "logrithmic" }, "Logrithmic scale"),

                ]),
                fieldset({}, [
                    legend("All row"),
                    input({ "type": "radio", "name": "graphAll", "id": "all", "checked": "checked", change: () => updateAllRow(chart, programmeCounts, false) }),
                    label({ "for": "all" }, "Show total for all programmes"),
                    input({ "type": "radio", "name": "graphAll", "id": "nqall", change: () => updateAllRow(chart, programmeCounts, true) }),
                    label({ "for": "nqall" }, "Show total for just main programmes"),
                ])
            ])
        ]),
    ]);
}

function createGraphPage(programmeCounts: Map<string, Map<BackupType, SizeCount>>) {
    base.appendChild(generateBarChart(programmeCounts));
    base.appendChild(generateGroupedBarChart(programmeCounts));
};

export default Object.assign(base, {
    init: createGraphPage
})