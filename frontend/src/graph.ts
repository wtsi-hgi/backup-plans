import { MainProgrammes } from "./consts.js";
import { div, p, h2, h3, br } from "./lib/html.js";
import type { SizeCount } from "./types.js";

// const TestProgrammes = ["All", "Unknown"];
const colourClasses = ["bar-one", "bar-two", "bar-three", "bar-four"];
const base = div();

function prepareData(programmeCounts: Map<string, Map<number, SizeCount>>) {
    const barChartData = new Map<string, number[]>();

    for (const programme of MainProgrammes) {
        const data = programmeCounts.get(programme)!

        let totalSize = 0;
        for (const i of [-1, 0, 1, 2]) {
            totalSize += Number(data.get(i)?.size) ?? 0;
        }

        const programmeFractions: number[] = [];
        for (const i of [-1, 0, 1, 2]) {
            programmeFractions.push(
                Math.round(100 * (Number(data.get(i)?.size) ?? 0) / totalSize)
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
            div({ "class": "graph" }, [
                h2("Data Fraction per Programme"), br(),
                MainProgrammes.map(programme => [
                    p(programme),
                    div({ "class": "graphRow" }, [...[0, 1, 2, 3].flatMap(i => div({
                        "style": "width:" + barChartData.get(programme)![i] + "%;",
                        "title": barChartData.get(programme)![i] + "%",
                        "class": colourClasses[i]
                    }))])
                ]), br(),
                h3("Key"),
                div({ "id": "graphKey" }, [
                    div("Unplanned"),
                    div("Backup"),
                    div("No Backup"),
                    div("Manual Backup")
                ]),
            ])
        ])
    );
}

export default Object.assign(base, {
    init: generateBarChart
})
