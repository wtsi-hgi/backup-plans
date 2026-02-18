import { div, h2, p, button, table, thead, tbody, th, td, tr, fieldset, legend, h1 } from "./lib/html.js";
import type { DirStats } from "./types.js";
import { getClaimStats } from "./rpc.js";
import { formatBytes, longAgoStr } from "./lib/utils.js";
import { BackupType } from "./consts.js";
import { svg, title, use } from "./lib/svg.js";
import { load } from './load.js';

const base = div();

export function initialiseClaimStats() {
    console.log("Initialising claim stats page");
    getClaimStats().then(claimStats => {
        base.replaceChildren(createClaimStatsPage(claimStats));
    });
}

function createClaimStatsPage(claimStats: DirStats[]) {
    return div({}, [
        div({ "class": "claimstats-container" }, [
            claimStats.length == 0 ? div(h2("You have not claimed any directories.")) :
                claimStats.map((dirStats) => fieldset({ "class": "userclaims" }, [
                    legend({ "class": "claimstats-legend" }, [h1(dirStats.Path), button({
                        "class": "load-button",
                        "click": () => load(dirStats.Path).then(() => {
                            window.scrollTo(0, 0);
                            document.getElementsByTagName("summary")[0].click();
                        })
                    }, svg([title("Go to"), use({ href: "#goto" })]))]),
                    div({}, [
                        div({ "class": "claiminfo" }, [
                            p("Last successful backup: " + (dirStats.BackupStatus.LastSuccess === "" ? "Pending" : longAgoStr(dirStats.BackupStatus.LastSuccess))),
                            p("Failures: " + dirStats.BackupStatus.Failures),
                        ]),
                        table({ "class": "summary" }, [
                            thead(tr({}, [
                                th("Match"),
                                th("Backup Type"),
                                th("Size"),
                                th("Count"),
                            ])),
                            tbody({}, [
                                dirStats.RuleStats.length > 0 ? dirStats.RuleStats.map((rule) => [
                                    tr({ "class": "rule-row" }, [
                                        td(rule.Match),
                                        td(rule.BackupType != null ? BackupType.from(rule.BackupType).optionLabel() : "Unplanned"),
                                        td(formatBytes(BigInt(rule.size))),
                                        td("" + rule.count)
                                    ])
                                ]) : tr(td({ "colspan": "4" }, "No rules or unplanned data."))
                            ])
                        ])
                    ])
                ]),
                )
        ])
    ])
}

initialiseClaimStats()
export default base
