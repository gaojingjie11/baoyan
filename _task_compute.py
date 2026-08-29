import json, datetime, subprocess

TODAY = subprocess.check_output(["date", "+%F"]).decode().strip()
now = datetime.date.fromisoformat(TODAY)
doy = now.timetuple().tm_yday

print("TODAY:", TODAY, "DOY:", doy)

with open("schools.json") as f:
    data = json.load(f)

# parse date helper
def parse_dt(s):
    if not s or s in ("待发布", "待官方公告", ""):
        return None
    s = s.strip()
    if " " in s:
        d, t = s.split(" ")
        if t == "24:00":
            d2 = datetime.date.fromisoformat(d) + datetime.timedelta(days=1)
            return datetime.datetime(d2.year, d2.month, d2.day, 0, 0)
        return datetime.datetime.strptime(s, "%Y-%m-%d %H:%M")
    return datetime.datetime.strptime(s, "%Y-%m-%d")


# 专业 -> (匹配函数, 该专业每批最多联网核验条数)
# 新增专业时只需在这里加一项，无需改动下面的分组逻辑。
MAJORS = {
    "计算机": (lambda m: m in ("", "计算机"), 14),
    "地理信息科学": (lambda m: m == "地理信息科学", 6),
    "工商管理": (lambda m: m == "工商管理", 6),
}

# Local status correction
flips = []
for s in data["schools"]:
    e = parse_dt(s.get("end"))
    if e is None:
        continue
    st = parse_dt(s.get("start"))
    st = st.date() if st else datetime.date.min
    if e.date() < now:
        if s["status"] != "已截止":
            flips.append((s["id"], s["school"], s["college"], s["status"], "→已截止"))
    elif st <= now <= e.date():
        if s["status"] != "报名中":
            flips.append((s["id"], s["school"], s["college"], s["status"], "→报名中"))
print("\n=== LOCAL STATUS CORRECTION (flips) ===")
for f in flips:
    print(f)

# Rolling windows
def window(lst, maxn):
    n = len(lst)
    if n == 0:
        return []
    off = doy % n
    sel = []
    for i in range(maxn):
        sel.append(lst[(off + i) % n])
    print(f"  n={n} offset={off} -> indices {off}..{off+maxn-1} (wrap handled)")
    return sel

# 按专业分组（通用循环，新增专业自动纳入）
groups = {}
for name, (matcher, maxn) in MAJORS.items():
    grp = [s for s in data["schools"] if matcher(s.get("major", ""))]
    dp = sorted([s for s in grp if s["status"] == "待发布"], key=lambda x: x["id"])
    groups[name] = (grp, dp, maxn)

print("\n=== COUNTS ===")
for name, (grp, dp, maxn) in groups.items():
    print(f"  {name}: total={len(grp)} 待发布={len(dp)}")

for name, (grp, dp, maxn) in groups.items():
    print(f"\n=== {name} WINDOW (max {maxn}) ===")
    for s in window(dp, maxn):
        print(f"  id={s['id']} {s['school']}·{s['college']}·{s['direction']}")

for name, (grp, dp, maxn) in groups.items():
    print(f"\n=== {name} 待发布 full list (idx:id) ===")
    for i, s in enumerate(dp):
        print(f"  [{i}] id={s['id']} {s['school']}·{s['college']}")
