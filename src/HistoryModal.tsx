import { useState, useEffect } from 'react';
import { getHistory, type HistoryRecord } from './history';
import type { ChannelType } from './App';
import './HistoryModal.css';

interface HistoryModalProps {
    isOpen: boolean;
    onClose: () => void;
    onDelete?: (id: string, selected: ChannelType, fire_id: string) => Promise<void>;
    onEdit?: (record: HistoryRecord, selected: ChannelType) => void;
}

export function HistoryModal({ isOpen, onClose, onDelete, onEdit }: HistoryModalProps) {
    const [selectedChannel, setSelectedChannel] = useState<ChannelType>('PLAY');
    const [histories, setHistories] = useState<HistoryRecord[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string>('');
    const [openMenuId, setOpenMenuId] = useState<string | null>(null);

    const channelOptions: { value: ChannelType; label: string }[] = [
        { value: 'PLAY', label: 'プレイ会' },
        { value: 'CREATE', label: '制作会' },
        { value: 'DRAFT', label: '運営用草稿チャンネル' },
    ];

    useEffect(() => {
        if (isOpen) {
            loadHistory();
        }
    }, [isOpen, selectedChannel]);

    const loadHistory = async () => {
        setLoading(true);
        setError('');
        try {
            const data = await getHistory(selectedChannel);
            setHistories(data);
        } catch (err) {
            setError(err instanceof Error ? err.message : '履歴の取得に失敗しました');
            console.error('履歴取得エラー:', err);
        } finally {
            setLoading(false);
        }
    };

    const formatDate = (timestamp: any) => {
        try {
            const date = timestamp?.toDate?.() || new Date(timestamp);
            return new Intl.DateTimeFormat('ja-JP', {
                year: 'numeric',
                month: '2-digit',
                day: '2-digit',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
            }).format(date);
        } catch {
            return '日時不明';
        }
    };

  if (!isOpen) return null;

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>📜 メッセージ履歴</h2>
          <button className="close-button" onClick={onClose} aria-label="閉じる">
            ×
          </button>
        </div>

        <div className="modal-body">
          <div className="channel-selector">
            <label htmlFor="channel-select">チャンネル:</label>
            <select
              id="channel-select"
              value={selectedChannel}
              onChange={(e) => setSelectedChannel(e.target.value as ChannelType)}
              className="channel-select"
            >
              {channelOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </div>

          {loading && <div className="loading-message">📥 読み込み中...</div>}

          {error && <div className="error-message">❌ {error}</div>}

          {!loading && !error && histories.length === 0 && (
            <div className="empty-message">📭 履歴がありません</div>
          )}

          {!loading && !error && histories.length > 0 && (
            <div className="history-list">
              {histories.map((record, index) => (
                <div key={`${record.id}-${index}`} className="history-item">
                  <div className="history-header">
                    <span className="history-date">{formatDate(record.createdAt)}</span>
                    <div className="menu-container">
                      <button 
                        className="menu-button"
                        onClick={() => setOpenMenuId(openMenuId === record.id ? null : record.id)}
                        aria-label="メニュー"
                      >
                        ⋮
                      </button>
                      {openMenuId === record.id && (
                        <div className="menu-dropdown">
                          <button 
                            className="menu-item history-delete-button"
                            onClick={async () => {
                              if (onDelete) {
                                await onDelete(record.id, selectedChannel, record.fire_id);
                                setOpenMenuId(null);
                                loadHistory();
                              }
                            }}
                          >
                            削除
                          </button>
                          <button 
                            className="menu-item history-edit-button"
                            onClick={() => {
                              if (onEdit) {
                                onEdit(record, selectedChannel);
                                setOpenMenuId(null);
                              }
                            }}
                          >
                            編集
                          </button>
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="history-message">{record.message}</div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
