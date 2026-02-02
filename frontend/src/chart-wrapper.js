// chart-wrapper.js
// @ts-ignore
import "./chart.umd.min.js"; // loads the UMD globally

export const Chart = window.Chart;
export const registerables = Chart.registry || []; // if needed
