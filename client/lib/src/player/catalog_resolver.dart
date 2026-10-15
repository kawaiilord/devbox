import 'dart:math' as math;

import 'package:flutter/foundation.dart';

import '../api/api_client.dart';
import '../catalog_models.dart';
import '../discovery_models.dart';
import '../models.dart';

// Keep season numbers and edition names: a fuzzy title match can play the
// wrong film. Only typography and explicitly marked quality labels are ignored.
String normalizeCatalogTitle(String title) => title
    .toLowerCase()
    .replaceAll(RegExp(r'[\[【(（](?:4k|1080p|720p|高清|超清)[\]】)）]'), '')
    .replaceAll(RegExp(r'[\s·・:：,，.。!！?？\-—_《》“”"\x27]'), '');

String catalogYear(String value) =>
    RegExp(r'(?:19|20)\d{2}').firstMatch(value)?.group(0) ?? '';

class WatchTarget {
  const WatchTarget({
    required this.title,
    required this.kind,
    this.year = '',
    this.original = '',
    this.poster = '',
    this.summary = '',
  });
  factory WatchTarget.discovery(DiscoveryItem item) => WatchTarget(
    title: item.title,
    kind: item.kind,
    year: catalogYear(item.date),
    original: item.original,
    poster: item.poster,
    summary: item.summary,
  );
  factory WatchTarget.film(CatalogFilm film) => WatchTarget(
    title: film.title,
    kind: film.kind,
    year: catalogYear(film.year),
    poster: film.poster,
    summary: film.summary,
  );
  final String title, kind, year, original, poster, summary;
  String get key => '$kind|$year|${normalizeCatalogTitle(title)}';

  int match(CatalogFilm film) {
    final name = normalizeCatalogTitle(film.title);
    if (name != normalizeCatalogTitle(title) &&
        (original.isEmpty || name != normalizeCatalogTitle(original))) {
      return 0;
    }
    if (kind.isNotEmpty && film.kind.isNotEmpty && kind != film.kind) return 0;
    final otherYear = catalogYear(film.year);
    if (year.isNotEmpty && otherYear.isNotEmpty && year != otherYear) return 0;
    return year.isNotEmpty && otherYear.isEmpty ? 1 : 2;
  }
}

class CatalogLine {
  const CatalogLine(this.film, this.group);
  final CatalogFilm film;
  final CatalogGroup group;
  String get id => '${film.sourceId}:${film.id}:${group.id}';
  String get label => '${film.sourceName} · ${group.name}';
}

String? episodeNumber(String name) {
  final text = name.trim().toLowerCase();
  final season = RegExp(r's(\d+)e(\d+)').firstMatch(text);
  if (season != null)
    return '${int.parse(season[1]!)}:${int.parse(season[2]!)}';
  final match =
      RegExp(r'^(?:第\s*)?(\d+)(?:\s*[集话期]|$)').firstMatch(text) ??
      RegExp(r'^ep?\.?\s*(\d+)$').firstMatch(text);
  return match == null ? null : '1:${int.parse(match[1]!)}';
}

// A missing episode is explicit, never silently restart episode one. Named
// extras/trailers are not mapped to numbered episodes by their list index.
int matchingEpisode(CatalogGroup from, int index, CatalogGroup to) {
  if (index < 0 || index >= from.episodes.length) return -1;
  final current = from.episodes[index];
  final number = episodeNumber(current.name);
  if (number != null) {
    return to.episodes.indexWhere((e) => episodeNumber(e.name) == number);
  }
  final name = normalizeCatalogTitle(current.name);
  final exact = to.episodes.indexWhere(
    (e) => normalizeCatalogTitle(e.name) == name,
  );
  if (exact >= 0) return exact;
  if (from.episodes.length == 1 && to.episodes.length == 1) return 0;
  return -1;
}

class CatalogResolver extends ChangeNotifier {
  CatalogResolver(this.api, this.session, this.target);
  final ApiClient api;
  final Session session;
  final WatchTarget target;
  final List<CatalogLine> lines = [];
  final Map<String, String> failures = {};
  final Set<String> _seen = {};
  bool loading = false;
  int done = 0, total = 0, _generation = 0;
  String? error;

  Future<void> resolve({List<CatalogFilm> seeds = const []}) async {
    final generation = ++_generation;
    lines.clear();
    failures.clear();
    _seen.clear();
    loading = true;
    error = null;
    done = 0;
    total = 0;
    notifyListeners();
    bool active() => generation == _generation;
    final deferred = <CatalogFilm>[];
    Future<void> detail(CatalogFilm candidate) async {
      if (!active() || !_seen.add('${candidate.sourceId}:${candidate.id}'))
        return;
      try {
        final film = await api.catalogDetail(session, candidate);
        if (!active()) return;
        // Explicit source selections (including Guoguo) have stable IDs;
        // discovered candidates must still match after their detail response.
        final explicit = seeds.any(
          (s) => s.id == film.id && s.sourceId == film.sourceId,
        );
        if (!explicit && target.match(film) == 0) return;
        for (final group in film.groups.where((g) => g.episodes.isNotEmpty)) {
          lines.add(CatalogLine(film, group));
        }
        notifyListeners();
      } catch (e) {
        if (active()) failures[candidate.sourceId] = '详情暂不可用';
      }
    }

    Future<void> details(List<CatalogFilm> films) async {
      var cursor = 0;
      await Future.wait(
        List.generate(math.min(4, films.length), (_) async {
          while (active() && cursor < films.length) {
            await detail(films[cursor++]);
          }
        }),
      );
    }

    try {
      // Do not wait for the search of unrelated sources to open a known film.
      if (seeds.isNotEmpty) {
        await detail(seeds.first);
        if (!active()) return;
      }
      if (target.kind == 'short' || target.kind == 'live') {
        await details(seeds.skip(1).toList());
        return;
      }
      final sources = (await api.catalogSources(session))
          .where((s) => s.kind == 'cms')
          .toList();
      if (!active()) return;
      sources.sort((a, b) {
        final now = DateTime.now().millisecondsSinceEpoch;
        final suspended = (a.suspendedUntil > now ? 1 : 0).compareTo(
          b.suspendedUntil > now ? 1 : 0,
        );
        if (suspended != 0) return suspended;
        return (a.latency > 0 ? a.latency : 9999).compareTo(
          b.latency > 0 ? b.latency : 9999,
        );
      });
      total = sources.length;
      notifyListeners();
      var cursor = 0;
      Future<void> worker() async {
        while (active() && cursor < sources.length) {
          final source = sources[cursor++];
          try {
            final result = await api.searchCatalog(
              session,
              source.id,
              target.title,
            );
            if (!active()) return;
            final matches = result.items
                .where((f) => target.match(f) > 0)
                .take(8)
                .toList();
            deferred.addAll(matches.where((f) => target.match(f) == 1));
            for (final candidate in matches.where(
              (f) => target.match(f) == 2,
            )) {
              await detail(candidate);
            }
          } catch (e) {
            if (active()) failures[source.id] = '片源暂不可用';
          } finally {
            if (active()) {
              done++;
              notifyListeners();
            }
          }
        }
      }

      await Future.wait([
        details(seeds.skip(1).take(7).toList()),
        ...List.generate(math.min(4, sources.length), (_) => worker()),
      ]);
      if (!active()) return;
      await details(deferred);
    } catch (e) {
      if (active()) error = e is ApiException ? e.message : '片源暂时连接失败';
    } finally {
      if (active()) {
        loading = false;
        if (lines.isEmpty) error ??= '当前片源暂未收录这部影片的可播放版本';
        notifyListeners();
      }
    }
  }

  @override
  void dispose() {
    _generation++;
    super.dispose();
  }
}
