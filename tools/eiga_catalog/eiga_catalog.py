#!/usr/bin/env python3
"""Collect movie/person catalog records from eiga.com with link edges.

This tool is intentionally outside the RenCrow runtime. It stores raw catalog
facts in SQLite/JSONL so later import or validation can be done explicitly.
"""

from __future__ import annotations

import argparse
import gzip
import html
import json
import re
import sqlite3
import sys
import time
import urllib.parse
import urllib.request
import urllib.robotparser
import xml.etree.ElementTree as ET
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Iterable


BASE_URL = "https://eiga.com"
DEFAULT_USER_AGENT = "RenCrowLocalResearch/0.1 (+local user-run; robots-aware)"
ROBOTS_URL = f"{BASE_URL}/robots.txt"
SITEMAP_INDEX_URL = f"{BASE_URL}/sitemap/index.xml.gz"


@dataclass
class LinkedPerson:
    person_id: str
    name: str
    url: str
    role: str = ""


@dataclass
class LinkedMovie:
    movie_id: str
    title: str
    url: str
    role: str = ""


@dataclass
class MovieRecord:
    movie_id: str
    title: str
    url: str
    synopsis: str = ""
    cast: list[LinkedPerson] = field(default_factory=list)
    staff: list[LinkedPerson] = field(default_factory=list)
    related_people: list[LinkedPerson] = field(default_factory=list)


@dataclass
class PersonRecord:
    person_id: str
    name: str
    url: str
    profile: dict[str, str] = field(default_factory=dict)
    biography: str = ""
    biography_movies: list[LinkedMovie] = field(default_factory=list)
    filmography: list[LinkedMovie] = field(default_factory=list)


def normalize_url(url: str) -> str:
    url = html.unescape(url.strip())
    return urllib.parse.urljoin(BASE_URL, url)


def entity_id(url: str, kind: str) -> str:
    match = re.search(rf"/{kind}/(\d+)/", url)
    return match.group(1) if match else ""


def clean_text(value: str) -> str:
    value = re.sub(r"<br\s*/?>", "\n", value, flags=re.I)
    value = re.sub(r"<[^>]+>", "", value)
    value = html.unescape(value)
    value = re.sub(r"[ \t\r\f\v]+", " ", value)
    value = re.sub(r"\n\s+", "\n", value)
    return value.strip()


def find_first(pattern: str, text: str, flags: int = re.S | re.I) -> str:
    match = re.search(pattern, text, flags)
    return match.group(1) if match else ""


def extract_ldjson(html_text: str) -> list[object]:
    items: list[object] = []
    for raw in re.findall(r'<script\s+type=["\']application/ld\+json["\'][^>]*>(.*?)</script>', html_text, re.S | re.I):
        try:
            parsed = json.loads(html.unescape(raw.strip()))
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, list):
            items.extend(parsed)
        else:
            items.append(parsed)
    return items


def person_from_schema(item: object, role: str = "") -> LinkedPerson | None:
    if not isinstance(item, dict):
        return None
    url = normalize_url(str(item.get("url") or ""))
    pid = entity_id(url, "person")
    name = clean_text(str(item.get("name") or ""))
    if not pid or not name:
        return None
    return LinkedPerson(person_id=pid, name=name, url=url, role=role)


def unique_people(items: Iterable[LinkedPerson]) -> list[LinkedPerson]:
    seen: set[tuple[str, str]] = set()
    out: list[LinkedPerson] = []
    for item in items:
        key = (item.person_id, item.role)
        if key in seen:
            continue
        seen.add(key)
        out.append(item)
    return out


def unique_movies(items: Iterable[LinkedMovie]) -> list[LinkedMovie]:
    seen: set[tuple[str, str]] = set()
    out: list[LinkedMovie] = []
    for item in items:
        key = (item.movie_id, item.role)
        if key in seen:
            continue
        seen.add(key)
        out.append(item)
    return out


def parse_movie(html_text: str, url: str) -> MovieRecord:
    url = normalize_url(url)
    movie_id = entity_id(url, "movie")
    title = clean_text(find_first(r'<h1[^>]*class=["\'][^"\']*page-title[^"\']*["\'][^>]*>(.*?)</h1>', html_text))
    synopsis = ""
    cast: list[LinkedPerson] = []
    staff: list[LinkedPerson] = []

    for item in extract_ldjson(html_text):
        if not isinstance(item, dict) or item.get("@type") != "Movie":
            continue
        title = title or clean_text(str(item.get("name") or ""))
        synopsis = clean_text(str(item.get("description") or ""))
        for actor in item.get("actor") or []:
            person = person_from_schema(actor, "出演")
            if person:
                cast.append(person)
        for director in item.get("director") or []:
            person = person_from_schema(director, "監督")
            if person:
                staff.append(person)

    if not synopsis:
        synopsis = clean_text(find_first(r'<h2[^>]*>\s*解説・あらすじ\s*</h2>\s*<p>(.*?)</p>', html_text))

    staff_block = find_first(r'<dl\s+class=["\']movie-staff["\'][^>]*>(.*?)</dl>', html_text)
    current_role = ""
    for tag, body in re.findall(r"<(dt|dd)[^>]*>(.*?)</\1>", staff_block, re.S | re.I):
        if tag.lower() == "dt":
            current_role = clean_text(body)
            continue
        href = find_first(r'href=["\'](/person/\d+/)["\']', body)
        name = clean_text(body)
        pid = entity_id(href, "person")
        if pid and name:
            staff.append(LinkedPerson(person_id=pid, name=name, url=normalize_url(href), role=current_role))

    cast_block = find_first(r'<ul\s+class=["\']movie-cast["\'][^>]*>(.*?)</ul>', html_text)
    for href, body in re.findall(r'<a[^>]+href=["\'](/person/\d+/)["\'][^>]*>(.*?)</a>', cast_block, re.S | re.I):
        pid = entity_id(href, "person")
        name = clean_text(body)
        if pid and name:
            cast.append(LinkedPerson(person_id=pid, name=name, url=normalize_url(href), role="出演"))

    related = unique_people([*cast, *staff])
    return MovieRecord(
        movie_id=movie_id,
        title=title,
        url=url,
        synopsis=synopsis,
        cast=unique_people(cast),
        staff=unique_people(staff),
        related_people=related,
    )


def parse_person(html_text: str, url: str, filmography_html: str = "") -> PersonRecord:
    url = normalize_url(url)
    person_id = entity_id(url, "person")
    name = clean_text(find_first(r'<h1[^>]*class=["\'][^"\']*page-title[^"\']*["\'][^>]*>(.*?)</h1>', html_text))
    profile: dict[str, str] = {}
    profile_block = find_first(r'<div\s+class=["\']profile["\'][^>]*>\s*<dl>(.*?)</dl>', html_text)
    current_key = ""
    for tag, attrs, body in re.findall(r"<(dt|dd)([^>]*)>(.*?)</\1>", profile_block, re.S | re.I):
        if tag.lower() == "dt":
            current_key = clean_text(body)
        elif current_key:
            if re.search(r'class=["\'][^"\']*\bsns\b', attrs, re.I):
                continue
            value = clean_text(body)
            if value:
                profile[current_key] = value
    bio_block = find_first(r'<div\s+class=["\']txt-block["\'][^>]*>\s*<p[^>]*class=["\']txt["\'][^>]*>(.*?)</p>', html_text)
    biography = clean_text(bio_block)
    biography_movies = movie_links_from_block(bio_block)
    filmography = movie_links_from_filmography(filmography_html) if filmography_html else []
    return PersonRecord(
        person_id=person_id,
        name=name,
        url=url,
        profile=profile,
        biography=biography,
        biography_movies=unique_movies(biography_movies),
        filmography=unique_movies(filmography),
    )


def movie_links_from_block(block: str) -> list[LinkedMovie]:
    out: list[LinkedMovie] = []
    for href, body in re.findall(r'<a[^>]+href=["\'](/movie/\d+/)["\'][^>]*>(.*?)</a>', block, re.S | re.I):
        mid = entity_id(href, "movie")
        title = clean_text(body)
        if mid and title:
            out.append(LinkedMovie(movie_id=mid, title=title, url=normalize_url(href)))
    return out


def movie_links_from_filmography(html_text: str) -> list[LinkedMovie]:
    out: list[LinkedMovie] = []
    for href, body in re.findall(r'<a[^>]+href=["\'](/movie/\d+/)["\'][^>]*>(.*?)</a>', html_text, re.S | re.I):
        mid = entity_id(href, "movie")
        title = clean_text(find_first(r'<p\s+class=["\']title["\'][^>]*>(.*?)</p>', body) or body)
        role = clean_text(find_first(r'<p\s+class=["\']label["\'][^>]*>(.*?)</p>', body))
        if mid and title:
            out.append(LinkedMovie(movie_id=mid, title=title, url=normalize_url(href), role=role))
    return out


class EigaStore:
    def __init__(self, db_path: Path, jsonl_path: Path):
        db_path.parent.mkdir(parents=True, exist_ok=True)
        jsonl_path.parent.mkdir(parents=True, exist_ok=True)
        self.conn = sqlite3.connect(db_path)
        self.jsonl_path = jsonl_path
        self._init_schema()

    def close(self) -> None:
        self.conn.close()

    def _init_schema(self) -> None:
        self.conn.executescript(
            """
            CREATE TABLE IF NOT EXISTS movies (
              movie_id TEXT PRIMARY KEY,
              title TEXT NOT NULL,
              url TEXT NOT NULL,
              synopsis TEXT,
              fetched_at TEXT DEFAULT CURRENT_TIMESTAMP
            );
            CREATE TABLE IF NOT EXISTS people (
              person_id TEXT PRIMARY KEY,
              name TEXT NOT NULL,
              url TEXT NOT NULL,
              profile_json TEXT,
              biography TEXT,
              fetched_at TEXT DEFAULT CURRENT_TIMESTAMP
            );
            CREATE TABLE IF NOT EXISTS movie_people (
              movie_id TEXT NOT NULL,
              person_id TEXT NOT NULL,
              role TEXT NOT NULL,
              source TEXT NOT NULL,
              movie_title TEXT,
              person_name TEXT,
              movie_url TEXT,
              person_url TEXT,
              PRIMARY KEY (movie_id, person_id, role, source)
            );
            CREATE TABLE IF NOT EXISTS fetch_log (
              url TEXT PRIMARY KEY,
              status TEXT NOT NULL,
              fetched_at TEXT DEFAULT CURRENT_TIMESTAMP,
              error TEXT
            );
            """
        )
        self.conn.commit()

    def append_jsonl(self, kind: str, payload: object) -> None:
        with self.jsonl_path.open("a", encoding="utf-8") as f:
            f.write(json.dumps({"kind": kind, **asdict(payload)}, ensure_ascii=False) + "\n")

    def save_movie(self, movie: MovieRecord) -> None:
        self.conn.execute(
            "INSERT OR REPLACE INTO movies(movie_id,title,url,synopsis) VALUES(?,?,?,?)",
            (movie.movie_id, movie.title, movie.url, movie.synopsis),
        )
        for person in movie.cast:
            self.save_edge(movie, person, person.role or "出演", "movie_cast")
        for person in movie.staff:
            self.save_edge(movie, person, person.role or "staff", "movie_staff")
        self.conn.commit()
        self.append_jsonl("movie", movie)

    def save_person(self, person: PersonRecord) -> None:
        self.conn.execute(
            "INSERT OR REPLACE INTO people(person_id,name,url,profile_json,biography) VALUES(?,?,?,?,?)",
            (person.person_id, person.name, person.url, json.dumps(person.profile, ensure_ascii=False), person.biography),
        )
        for movie in person.biography_movies:
            self.save_person_movie_edge(person, movie, movie.role or "略歴内関連作", "person_biography")
        for movie in person.filmography:
            self.save_person_movie_edge(person, movie, movie.role or "関連作品", "person_filmography")
        self.conn.commit()
        self.append_jsonl("person", person)

    def save_edge(self, movie: MovieRecord, person: LinkedPerson, role: str, source: str) -> None:
        self.conn.execute(
            """INSERT OR REPLACE INTO movie_people
               (movie_id,person_id,role,source,movie_title,person_name,movie_url,person_url)
               VALUES(?,?,?,?,?,?,?,?)""",
            (movie.movie_id, person.person_id, role, source, movie.title, person.name, movie.url, person.url),
        )

    def save_person_movie_edge(self, person: PersonRecord, movie: LinkedMovie, role: str, source: str) -> None:
        self.conn.execute(
            """INSERT OR REPLACE INTO movie_people
               (movie_id,person_id,role,source,movie_title,person_name,movie_url,person_url)
               VALUES(?,?,?,?,?,?,?,?)""",
            (movie.movie_id, person.person_id, role, source, movie.title, person.name, movie.url, person.url),
        )

    def mark_fetch(self, url: str, status: str, error: str = "") -> None:
        self.conn.execute(
            "INSERT OR REPLACE INTO fetch_log(url,status,error) VALUES(?,?,?)",
            (url, status, error),
        )
        self.conn.commit()


class Fetcher:
    def __init__(self, user_agent: str, delay: float, timeout: float):
        self.user_agent = user_agent
        self.delay = delay
        self.timeout = timeout
        self.last_fetch = 0.0
        self.robots = urllib.robotparser.RobotFileParser()
        self.robots.set_url(ROBOTS_URL)
        self.robots.read()

    def allowed(self, url: str) -> bool:
        return self.robots.can_fetch(self.user_agent, url)

    def get(self, url: str) -> str:
        url = normalize_url(url)
        if not self.allowed(url):
            raise RuntimeError(f"robots.txt disallows fetch: {url}")
        wait = self.delay - (time.monotonic() - self.last_fetch)
        if wait > 0:
            time.sleep(wait)
        req = urllib.request.Request(url, headers={"User-Agent": self.user_agent})
        with urllib.request.urlopen(req, timeout=self.timeout) as res:
            data = res.read()
        self.last_fetch = time.monotonic()
        return data.decode("utf-8", "replace")

    def get_bytes(self, url: str) -> bytes:
        if not self.allowed(url):
            raise RuntimeError(f"robots.txt disallows fetch: {url}")
        req = urllib.request.Request(url, headers={"User-Agent": self.user_agent})
        with urllib.request.urlopen(req, timeout=self.timeout) as res:
            return res.read()


def sitemap_locations(fetcher: Fetcher, url: str = SITEMAP_INDEX_URL) -> list[str]:
    data = fetcher.get_bytes(url)
    text = gzip.decompress(data).decode("utf-8", "replace") if data[:2] == b"\x1f\x8b" else data.decode("utf-8", "replace")
    root = ET.fromstring(text)
    ns = {"sm": "http://www.sitemaps.org/schemas/sitemap/0.9"}
    return [el.text.strip() for el in root.findall(".//sm:loc", ns) if el.text]


def discover_catalog_urls(fetcher: Fetcher, max_sitemaps: int = 0) -> list[str]:
    out: list[str] = []
    sitemap_urls = sitemap_locations(fetcher)
    if max_sitemaps > 0:
        sitemap_urls = sitemap_urls[:max_sitemaps]
    for sitemap_url in sitemap_urls:
        for loc in sitemap_locations(fetcher, sitemap_url):
            if re.search(r"https://eiga\.com/(movie|person)/\d+/$", loc) and fetcher.allowed(loc):
                out.append(loc)
    return sorted(set(out))


def crawl(args: argparse.Namespace) -> int:
    fetcher = Fetcher(args.user_agent, args.delay, args.timeout)
    queue: list[str] = [normalize_url(url) for url in args.seed_url]
    if args.all_from_sitemap:
        queue.extend(discover_catalog_urls(fetcher, args.max_sitemaps))
    queue = sorted(set(queue))
    if args.dry_run:
        for url in queue[: args.max_pages or len(queue)]:
            print(url)
        print(f"dry_run_total={len(queue)}")
        return 0
    if args.max_pages <= 0:
        print("--max-pages is required unless --dry-run is used", file=sys.stderr)
        return 2

    store = EigaStore(args.db, args.jsonl)
    seen: set[str] = set()
    processed = 0
    try:
        while queue and processed < args.max_pages:
            url = queue.pop(0)
            if url in seen:
                continue
            seen.add(url)
            try:
                body = fetcher.get(url)
                if re.search(r"/movie/\d+/$", url):
                    movie = parse_movie(body, url)
                    store.save_movie(movie)
                    if args.follow_links:
                        queue.extend(p.url for p in movie.related_people if p.url not in seen)
                elif re.search(r"/person/\d+/$", url):
                    filmography_html = ""
                    if args.include_person_filmography:
                        filmography_url = url.rstrip("/") + "/movie/"
                        if fetcher.allowed(filmography_url):
                            filmography_html = fetcher.get(filmography_url)
                    person = parse_person(body, url, filmography_html)
                    store.save_person(person)
                    if args.follow_links:
                        queue.extend(m.url for m in [*person.biography_movies, *person.filmography] if m.url not in seen)
                else:
                    continue
                store.mark_fetch(url, "ok")
                processed += 1
                print(f"ok {processed}/{args.max_pages}: {url}", flush=True)
            except Exception as exc:  # noqa: BLE001 - CLI records and continues.
                store.mark_fetch(url, "error", str(exc))
                print(f"error: {url}: {exc}", file=sys.stderr, flush=True)
                processed += 1
    finally:
        store.close()
    return 0


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Collect eiga.com movie/person records and cross-links.")
    parser.add_argument("--seed-url", action="append", default=[], help="Movie/person URL to start from. Repeatable.")
    parser.add_argument("--all-from-sitemap", action="store_true", help="Discover movie/person URLs from sitemap.")
    parser.add_argument("--max-sitemaps", type=int, default=0, help="Limit sitemap files for discovery; 0 means all.")
    parser.add_argument("--max-pages", type=int, default=0, help="Maximum pages to fetch. Required unless --dry-run.")
    parser.add_argument("--follow-links", action="store_true", help="Follow movie/person links discovered from parsed pages.")
    parser.add_argument("--include-person-filmography", action="store_true", help="Fetch /person/{id}/movie/ for full filmography links.")
    parser.add_argument("--output-dir", type=Path, default=Path("tmp/eiga_catalog"), help="Output directory.")
    parser.add_argument("--db", type=Path, default=None, help="SQLite output path.")
    parser.add_argument("--jsonl", type=Path, default=None, help="JSONL output path.")
    parser.add_argument("--delay", type=float, default=2.0, help="Minimum seconds between page fetches.")
    parser.add_argument("--timeout", type=float, default=20.0, help="HTTP timeout seconds.")
    parser.add_argument("--user-agent", default=DEFAULT_USER_AGENT, help="HTTP User-Agent.")
    parser.add_argument("--dry-run", action="store_true", help="Print planned URLs without fetching entity pages.")
    args = parser.parse_args(argv)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    args.db = args.db or args.output_dir / "eiga_catalog.sqlite"
    args.jsonl = args.jsonl or args.output_dir / "eiga_catalog.jsonl"
    if not args.seed_url and not args.all_from_sitemap:
        parser.error("provide --seed-url or --all-from-sitemap")
    return args


def main(argv: list[str]) -> int:
    return crawl(parse_args(argv))


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
