#!/usr/bin/env python3
# Safe batch updater for schools.json.
# Only whitelisted fields (start,end,status,site,source,remark) may be changed.
# Protected fields (id,type,school,college,direction,major,admit) are never touched.
# Usage: python3 _apply_updates.py '{"<today>": "2026-09-06", "updates": {"17": {"status":"已截止"}, ...}}'
import json, sys

PATH = "schools.json"
WL = {"start", "end", "status", "site", "source", "remark"}

def main():
    spec = json.loads(sys.argv[1])
    today = spec["today"]
    updates = spec["updates"]
    data = json.load(open(PATH, encoding="utf-8"))
    schools = data["schools"]
    by_id = {e["id"]: e for e in schools}
    changed = []
    for sid, fields in updates.items():
        sid = int(sid)
        e = by_id.get(sid)
        if e is None:
            print(f"  WARN id={sid} not found, skip")
            continue
        for k, v in fields.items():
            if k not in WL:
                print(f"  BLOCKED protected/illegal field '{k}' on id={sid}, skip field")
                continue
            if e.get(k) == v:
                continue
            e[k] = v
        changed.append(sid)
    data["updated_at"] = today
    with open(PATH, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print(f"Applied updates to {len(changed)} entries; updated_at={today}")
    print("changed ids:", sorted(changed))

if __name__ == "__main__":
    main()
