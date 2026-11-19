import type { FeatureAssociationIndex, FeatureTopologyAssociation } from "../../types";

const kinds = ["FACE", "EDGE", "VERTEX"] as const;
const statuses = ["MAPPED", "MAPPING_MISSING"] as const;

export function decodeFeatureAssociationTable(value: unknown): FeatureAssociationIndex {
    if (typeof value !== "object" || value === null) throw new Error("Invalid display association table");
    const table = value as Record<string, unknown>;
    if (table.version !== 1 || !Array.isArray(table.symbols) || !Array.isArray(table.features) ||
        !Array.isArray(table.rows) || (table.lists !== undefined && !Array.isArray(table.lists)) ||
        (table.members !== undefined && !Array.isArray(table.members))) {
        throw new Error("Invalid display association table");
    }
    const symbols = table.symbols;
    const ordinal = (value: unknown, limit: number): value is number =>
        typeof value === "number" && Number.isInteger(value) && value >= 0 && value < limit;
    const symbol = (id: unknown): string => {
        if (!ordinal(id, symbols.length + 1) || id === 0 || typeof symbols[id - 1] !== "string" || !symbols[id - 1]) {
            throw new Error("Invalid display association symbol");
        }
        return symbols[id - 1];
    };
    const idLists: string[][] = [[], ...(table.lists ?? []).map((list: unknown) => {
        if (!Array.isArray(list)) throw new Error("Invalid display association list");
        return list.map(symbol);
    })];
    const memberLists: {patternId: string; slot: number}[][] = [[], ...(table.members ?? []).map((pairs: unknown) => {
        if (!Array.isArray(pairs) || pairs.length % 2 !== 0) throw new Error("Invalid display association members");
        const members: {patternId: string; slot: number}[] = [];
        for (let i = 0; i < pairs.length; i += 2) {
            const slot = pairs[i + 1];
            if (!ordinal(slot, 0x100000000)) throw new Error("Invalid display association member slot");
            members.push({ patternId: symbol(pairs[i]), slot });
        }
        return members;
    })];
    const locators = new Set<number>();
    const elements = table.rows.map((row: unknown): FeatureTopologyAssociation => {
        if (!Array.isArray(row) || row.length !== 8) throw new Error("Invalid display association row");
        const [kind, localId, status, origins, primary, modifiers, supports, members] = row;
        if (!ordinal(kind, kinds.length) || !ordinal(localId, 0x100000000) || localId === 0 ||
            !ordinal(status, statuses.length) || !ordinal(origins, idLists.length) || !ordinal(primary, idLists.length) ||
            !ordinal(modifiers, idLists.length) || !ordinal(supports, idLists.length) || !ordinal(members, memberLists.length)) {
            throw new Error("Invalid display association row");
        }
        const locator = kind * 0x100000000 + localId;
        if (locators.has(locator)) throw new Error("Duplicate display association locator");
        locators.add(locator);
        return {
            kind: kinds[kind], localId, status: statuses[status], origins: idLists[origins], primary: idLists[primary],
            modifiers: idLists[modifiers], supports: idLists[supports], members: memberLists[members],
        };
    });
    return { features: table.features.map(symbol), elements };
}
