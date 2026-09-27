export const occColor = (v: number) =>
  v >= 90 ? "#C8352E" : v >= 70 ? "#E07B39" : v >= 40 ? "#E3B448" : "#7FB8A4";
export const occWord = (v: number) => (v >= 90 ? "Crush" : v >= 70 ? "Busy" : v >= 40 ? "Moderate" : "Comfortable");
export const profileLabel: Record<string, string> = {
  fastest: "Fastest", least_crowded: "Least crowded", fewest_transfers: "Fewest changes", balanced: "Balanced",
};
