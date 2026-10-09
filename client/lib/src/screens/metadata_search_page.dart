import 'package:flutter/material.dart';

import '../api/api_client.dart';
import '../models.dart';
import 'reviews_page.dart';

class MetadataSearchPage extends StatefulWidget {
  const MetadataSearchPage({
    super.key,
    required this.api,
    required this.session,
  });

  final ApiClient api;
  final Session session;

  @override
  State<MetadataSearchPage> createState() => _MetadataSearchPageState();
}

class _MetadataSearchPageState extends State<MetadataSearchPage> {
  final _query = TextEditingController();
  List<MetadataResult> _results = const [];
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _query.dispose();
    super.dispose();
  }

  Future<void> _search() async {
    final query = _query.text.trim();
    if (query.isEmpty) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final results = await widget.api.searchMetadata(widget.session, query);
      if (mounted) setState(() => _results = results);
    } catch (exception) {
      if (mounted) setState(() => _error = exception.toString());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('影视元数据搜索')),
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.all(16),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _query,
                    decoration: const InputDecoration(
                      labelText: '电影或剧集名称',
                      prefixIcon: Icon(Icons.search_rounded),
                    ),
                    onSubmitted: (_) => _search(),
                  ),
                ),
                const SizedBox(width: 10),
                FilledButton(
                  onPressed: _busy ? null : _search,
                  child: const Text('搜索'),
                ),
              ],
            ),
          ),
          if (_busy) const LinearProgressIndicator(),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.all(16),
              child: Text(
                _error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
          Expanded(
            child: _results.isEmpty && !_busy
                ? const Center(child: Text('输入名称搜索电影或剧集。'))
                : ListView.separated(
                    padding: const EdgeInsets.all(16),
                    itemCount: _results.length,
                    separatorBuilder: (_, _) => const SizedBox(height: 10),
                    itemBuilder: (context, index) {
                      final result = _results[index];
                      return Card(
                        child: ListTile(
                          contentPadding: const EdgeInsets.all(12),
                          leading: SizedBox(
                            width: 48,
                            height: 72,
                            child: result.posterUrl.isEmpty
                                ? const Icon(Icons.movie_outlined)
                                : Image.network(
                                    result.posterUrl,
                                    fit: BoxFit.cover,
                                    errorBuilder: (_, _, _) =>
                                        const Icon(Icons.broken_image_outlined),
                                  ),
                          ),
                          title: Text(result.title),
                          subtitle: Text(
                            '${result.mediaType == 'movie' ? '电影' : '剧集'} · ${result.releaseDate.isEmpty ? '日期未知' : result.releaseDate} · ${result.rating.toStringAsFixed(1)}\n${result.overview}',
                            maxLines: 4,
                            overflow: TextOverflow.ellipsis,
                          ),
                          isThreeLine: true,
                          trailing: const Icon(Icons.chevron_right_rounded),
                          onTap: () => Navigator.of(context).push(
                            MaterialPageRoute<void>(
                              builder: (_) => ReviewsPage(
                                api: widget.api,
                                session: widget.session,
                                targetType: result.mediaType,
                                targetId: result.id.toString(),
                                title: result.title,
                              ),
                            ),
                          ),
                        ),
                      );
                    },
                  ),
          ),
        ],
      ),
    );
  }
}
