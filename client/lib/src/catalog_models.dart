class CatalogSource {
  CatalogSource.fromJson(Map<String, dynamic> j)
    : id = j['id'] as String,
      name = j['name'] as String,
      kind = j['kind'] as String,
      latency = (j['health']?['latency_ms'] as num?)?.toInt() ?? 0,
      suspendedUntil = (j['health']?['suspended_until'] as num?)?.toInt() ?? 0;
  final String id, name, kind;
  final int latency, suspendedUntil;
}

class CatalogFilm {
  CatalogFilm.fromJson(Map<String, dynamic> j)
    : id = j['id'] as String,
      sourceId = j['source_id'] as String,
      sourceName = j['source_name'] as String,
      title = j['title'] as String,
      year = j['year']?.toString() ?? '',
      kind = j['kind']?.toString() ?? '',
      poster = j['poster']?.toString() ?? '',
      summary = j['summary']?.toString() ?? '',
      actors = j['actors']?.toString() ?? '',
      remarks = j['remarks']?.toString() ?? '',
      groupKey = j['group_key'] as String,
      groups = (j['groups'] as List? ?? [])
          .map((v) => CatalogGroup.fromJson(v as Map<String, dynamic>))
          .toList();
  final String id,
      sourceId,
      sourceName,
      title,
      year,
      kind,
      poster,
      summary,
      actors,
      remarks,
      groupKey;
  final List<CatalogGroup> groups;
  String get kindLabel => switch (kind) {
    'tv' => '剧集',
    'anime' => '动画',
    'variety' => '综艺',
    'live' => '直播',
    _ => '电影',
  };
}

class CatalogGroup {
  CatalogGroup.fromJson(Map<String, dynamic> j)
    : id = j['id'] as String,
      name = j['name'] as String,
      episodes = (j['episodes'] as List)
          .map((v) => CatalogEpisode.fromJson(v as Map<String, dynamic>))
          .toList();
  final String id, name;
  final List<CatalogEpisode> episodes;
}

class CatalogEpisode {
  CatalogEpisode.fromJson(Map<String, dynamic> j)
    : id = j['id'] as String,
      name = j['name'] as String;
  final String id, name;
}

class CatalogSearch {
  CatalogSearch.fromJson(Map<String, dynamic> j)
    : items = (j['items'] as List)
          .map((v) => CatalogFilm.fromJson(v as Map<String, dynamic>))
          .toList(),
      hasMore = j['has_more'] == true;
  final List<CatalogFilm> items;
  final bool hasMore;
}

List<List<CatalogFilm>> groupCatalogFilms(Iterable<CatalogFilm> films) {
  final groups = <String, List<CatalogFilm>>{};
  final seen = <String>{};
  for (final film in films) {
    if (seen.add('${film.sourceId}:${film.id}')) {
      groups.putIfAbsent(film.groupKey, () => []).add(film);
    }
  }
  return groups.values.toList();
}
