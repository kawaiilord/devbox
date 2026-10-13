class DiscoveryItem {
  const DiscoveryItem({
    required this.id,
    required this.title,
    required this.kind,
    this.original = '',
    this.rating = 0,
    this.votes = 0,
    this.poster = '',
    this.url = '',
    this.date = '',
    this.summary = '',
    this.episodes = '',
    this.weekday = 0,
    this.rank = 0,
    this.isNew = false,
  });
  final String id, title, original, kind, poster, url, date, summary, episodes;
  final double rating;
  final int votes, weekday, rank;
  final bool isNew;
  String get kindLabel => switch (kind) {
    'tv' => '剧集',
    'anime' => '动画',
    _ => '电影',
  };
  factory DiscoveryItem.fromJson(Map<String, dynamic> j) => DiscoveryItem(
    id: j['id'] as String,
    title: j['title'] as String,
    kind: j['kind'] as String,
    original: j['original']?.toString() ?? '',
    rating: (j['rating'] as num?)?.toDouble() ?? 0,
    votes: (j['votes'] as num?)?.toInt() ?? 0,
    poster: j['poster']?.toString() ?? '',
    url: j['url']?.toString() ?? '',
    date: j['date']?.toString() ?? '',
    summary: j['summary']?.toString() ?? '',
    episodes: j['episodes']?.toString() ?? '',
    weekday: (j['weekday'] as num?)?.toInt() ?? 0,
    rank: (j['rank'] as num?)?.toInt() ?? 0,
    isNew: j['is_new'] == true,
  );
}

class DiscoveryFeed {
  const DiscoveryFeed({
    required this.items,
    required this.source,
    required this.page,
    required this.fetchedAt,
    this.hasMore = false,
    this.stale = false,
    this.weekday = 0,
  });
  final List<DiscoveryItem> items;
  final String source;
  final int page, weekday;
  final DateTime fetchedAt;
  final bool hasMore, stale;
  factory DiscoveryFeed.fromJson(Map<String, dynamic> j) => DiscoveryFeed(
    items: (j['items'] as List<dynamic>)
        .map((v) => DiscoveryItem.fromJson(v as Map<String, dynamic>))
        .toList(),
    source: j['source']?.toString() ?? '',
    page: (j['page'] as num).toInt(),
    weekday: (j['weekday'] as num?)?.toInt() ?? 0,
    fetchedAt: DateTime.fromMillisecondsSinceEpoch(
      (j['fetched_at'] as num).toInt(),
    ),
    hasMore: j['has_more'] == true,
    stale: j['stale'] == true,
  );
}
