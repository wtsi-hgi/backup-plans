import { MainProgrammes } from "./consts.js";
import { div, p, h2, h3, br, span } from "./lib/html.js";
import type { SizeCount } from "./types.js";

// const MainProgrammes = ["All", "Unknown"];
const colourClasses = ["bar-one", "bar-two", "bar-three", "bar-four"];
const base = div();

// TODO:
// 1) Investigate why humgen's Manual Backup % is calculated as 0, when it should be 1 to make all numbers add to 100.
// 3) Add percentages to bars somehow (potentially an onhover effect if it looks too noisy)
// 5) Light mode fixes
function prepareData(programmeCounts: Map<string, Map<number, SizeCount>>) {
    const barChartData = new Map<string, number[]>();

    for (const programme of MainProgrammes) {
        const data = programmeCounts.get(programme)!

        let totalSize = 0;
        for (const i of [-1, 0, 1, 2]) {
            totalSize += Number(data.get(i)?.size || 0);
        }

        console.log("totalSize for programme:", programme, totalSize);

        const programmeFractions: number[] = [];
        for (const i of [-1, 0, 1, 2]) {
            programmeFractions.push(
                Math.round(100 * (Number(data.get(i)?.size) || 0) / totalSize)
            );
        }

        barChartData.set(programme, programmeFractions);
    }

    console.log(barChartData);
    return barChartData
}

function generateBarChart(programmeCounts: Map<string, Map<number, SizeCount>>) {
    const barChartData = prepareData(programmeCounts);

    base.replaceChildren(
        div({ "class": "graph-container" }, [
            h2("Data Fraction per Programme"),
            div({ "id": "graphKey" }, [
                div("Unplanned"),
                div("No Backup"),
                div("Backup"),
                div("Manual Backup")
            ]), br(),
            div({ "class": "graph-grid" }, [
                div({ "class": "graphAxisY" }, [
                    div(),
                    MainProgrammes.map(programme => [div(programme)]),
                    div()
                ]),
                div({ "class": "graph-wrapper" }, [
                    div({ "class": "graph" }, [
                        MainProgrammes.map(programme => [
                            div({ "class": "graphRow" }, [...[0, 1, 2, 3].flatMap(i => div({
                                "style": "width:" + barChartData.get(programme)![i] + "%;",
                                "title": barChartData.get(programme)![i] + "%",
                                "class": colourClasses[i]
                            }))])
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
    );
}

export default Object.assign(base, {
    init: generateBarChart
})
