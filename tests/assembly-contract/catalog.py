"""Compose production semantics with test-only evidence; no duplicated matrix."""
import argparse
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent

def load_catalog():
    index = json.loads((HERE / "catalog.json").read_text())
    production = json.loads((HERE / index["production"]).read_text())
    evidence = json.loads((HERE / index["evidence"]).read_text())
    return compose_catalog(production, evidence)

def compose_catalog(production, evidence):
    # Detached data: callers/tests cannot mutate the production authority.
    production = json.loads(json.dumps(production))
    cases = []
    for declared in evidence["cases"]:
        case = dict(declared)
        binding = case.pop("bindingId")
        if binding not in evidence["bindings"]:
            raise ValueError(f"{case['caseId']}: missing binding {binding}")
        overlap = case.keys() & evidence["bindings"][binding].keys()
        if overlap:
            raise ValueError(f"{case['caseId']}: duplicated binding fields {overlap}")
        cases.append({**evidence["bindings"][binding], **case})
    if {case["bindingId"] for case in evidence["cases"]} != evidence["bindings"].keys():
        raise ValueError("unused test binding")
    identities = {c["capabilityId"] for c in production["capabilities"]}
    if identities != evidence["capabilityEvidence"].keys():
        raise ValueError("production/test capability references differ")
    for capability in production["capabilities"]:
        ident = capability["capabilityId"]
        if capability.keys() & evidence["capabilityEvidence"][ident].keys():
            raise ValueError(f"{ident}: test evidence overwrites production semantics")
        capability.update(evidence["capabilityEvidence"][ident])
        capability["caseIds"] = [case["caseId"] for case in cases if ident in case["capabilityIds"]]
    return {**production, "layers": evidence["layers"], "implementationProfiles": evidence["implementationProfiles"], "cases": cases}

if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--capability")
    p.add_argument("--family")
    p.add_argument("--case")
    p.add_argument("--expanded", action="store_true", help="explicit full export for test adapters")
    args = p.parse_args()
    catalog = load_catalog()
    if args.case:
        output = [c for c in catalog["cases"] if c["caseId"] == args.case]
    elif args.capability or args.family:
        output = [c for c in catalog["capabilities"] if (not args.capability or c["capabilityId"] == args.capability) and (not args.family or c["family"] == args.family)]
    elif args.expanded:
        output = catalog
    else:
        output = {"contractVersion":catalog["contractVersion"],"families":catalog["families"],"capabilities":len(catalog["capabilities"]),"cases":len(catalog["cases"]),"query":"--capability ID / --family NAME / --case ID; --expanded for adapters"}
    if not output: p.error("selection matched no declaration")
    print(json.dumps(output, ensure_ascii=False, indent=2))
