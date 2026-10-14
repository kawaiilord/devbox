import 'catalog_models.dart';

class ShortDramaFeed {
  ShortDramaFeed.fromJson(Map<String, dynamic> json)
    : items = (json['items'] as List? ?? [])
          .map((v) => CatalogFilm.fromJson(v as Map<String, dynamic>))
          .toList(),
      hasMore = json['has_more'] == true,
      offset = (json['offset'] as num?)?.toInt() ?? 0,
      page = (json['page'] as num?)?.toInt() ?? 1,
      session = json['session']?.toString() ?? '',
      warning = json['warning']?.toString() ?? '';
  final List<CatalogFilm> items;
  final bool hasMore;
  final int offset, page;
  final String session, warning;
}
