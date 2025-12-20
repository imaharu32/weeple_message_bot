import { useState } from 'react'
import './App.css'
import { collection, addDoc, deleteDoc, doc} from "firebase/firestore"; 
import { db } from "./firebase";
import { HistoryModal } from "./HistoryModal";

export type ChannelType = 'PLAY' | 'CREATE' | 'DRAFT'
interface Option {
  label: string
  url: string
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
  channelType: ChannelType
}
const comment = 'メッセージに@Peepleとかを入れてもメンションができません。以下の<@&role_id>を代わりに使ってください。'
const peeple_role_comment = '@Peeple:　　<@&1122017976356434066>'
const leeple_role = '@Leeple:　　<@&1140645113467523087>'
const play_url = import.meta.env.VITE_PLAY_WEBHOOK_URL
const create_url = import.meta.env.VITE_CREATE_WEBHOOK_URL
const draft_url = import.meta.env.VITE_DRAFT_WEBHOOK_URL

async function addMessage(message: string, id: string, type: ChannelType) {
  const db_type = type + "_messages"
  try {
    await addDoc(collection(db, db_type), {
      message: message,
      id: id,
      createdAt: new Date()
    });
    console.log("メッセージをDBに保存しました");
  } catch (e) {
    console.error("Error adding document: ", e);
  }
}

function App() {
  
  const [loading, setLoading] = useState(false)
  const [response, setResponse] = useState<string>('')
  const [error, setError] = useState<string>('')
  const [message, setMessage] = useState<string>('')
  const [selectedOption, setSelectedOption] = useState<string>('0')
  const [isHistoryModalOpen, setIsHistoryModalOpen] = useState(false)

  const post_options: Option[] = [
    { label: 'プレイ会', url: play_url, method: 'POST', channelType: 'PLAY' },
    { label: '制作会', url: create_url, method: 'POST', channelType: 'CREATE' },
    { label: '運営用草稿チャンネル', url: draft_url, method: 'POST', channelType: 'DRAFT' },
  ]
  const channelMap: Record<ChannelType, Option> = {
    PLAY: post_options[0],
    CREATE: post_options[1],
    DRAFT: post_options[2],
  };
  const handleRequest = async (option: Option, message: string) => {
    setLoading(true)
    setError('')
    setResponse('')

    if (!option.url) {
      setLoading(false)
      setError('送信先URLが設定されていません。環境変数をご確認ください。')
      return
    }

    try {
      const response = await fetch(option.url + '?wait=true', {
        method: option.method,
        headers: {
          'Content-Type': 'application/json',
        },
        ...(option.method !== 'GET' && {
          body: JSON.stringify({content: message })
        })
      })

      if (!response.ok) {
        console.error('Response not ok:', response)
        throw new Error(`HTTP Error: ${response.status}`)
      }
      const responseData = await response.json()
      const id = responseData.id || 'unknown_id'
      await addMessage(message, id, option.channelType)
      setResponse("送信に成功しました！")
    } catch (err) {
      console.error('Request failed:')
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="App">
      <h1>Weeple botメッセージ送信ツール</h1>
      <h2>{comment}</h2>

      <h2>{peeple_role_comment}</h2>
      <h2>{leeple_role}</h2>
      
      <button 
        className="history-button"
        onClick={() => setIsHistoryModalOpen(true)}
      >
        メッセージ履歴
      </button>

      <div className="message-form">
        <div className="form-group">
          <label htmlFor="destination">送信先を選択:</label>
          <select
            id="destination"
            value={selectedOption}
            onChange={(e) => setSelectedOption(e.target.value)}
            disabled={loading}
            className="select-input"
          >
            <option value="">-- 送信先を選択してください --</option>
            {post_options.map((option, index) => (
              <option key={index} value={index.toString()}>
                {option.label}
              </option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="message">メッセージ内容:</label>
          <textarea
            id="message"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            disabled={loading}
            placeholder="ここにメッセージを入力してください..."
            className="textarea-input"
            rows={5}
          />
        </div>

        <button
          onClick={() => {
            if (!selectedOption) {
              setError('送信先を選択してください')
              return
            }
            if (!message.trim()) {
              setError('メッセージを入力してください')
              return
            }
            handleRequest(post_options[parseInt(selectedOption)], message)
          }}
          disabled={loading || !selectedOption || !message.trim()}
          className="send-button"
        >
          {loading ? 'メッセージ送信中...' : 'メッセージを送信'}
        </button>
      </div>

      {loading && <p className="loading">リクエスト送信中...</p>}

      {error && <div className="error">エラー: {error}</div>}

      {response && (
        <div className="response">
          <div>{response}</div>
        </div>
      )}

      <HistoryModal 
        isOpen={isHistoryModalOpen} 
        onClose={() => setIsHistoryModalOpen(false)} 
        onDelete={async (id: string, selectedChannel: ChannelType, fire_id: string) => {
          const option = channelMap[selectedChannel];
          if (!option?.url) {
            setError('削除先URLが設定されていません');
            return;
          }
          const delurl = option.url + '/messages/' + id
          try {
            const response = await fetch(delurl, {
              method: 'DELETE',
            })
            if (!response.ok) {
              console.error('Response not ok:', response)
              throw new Error(`HTTP Error: ${response.status}`)
            }
            await deleteDoc(doc(db, selectedChannel + "_messages",fire_id ))
          } catch (err) {
            console.error('Request failed:', delurl)
            setError(err instanceof Error ? err.message : 'Unknown error')
          } finally {
            setLoading(false)
            
          }
          
        }}
      />
    </div>
  )
}

export default App
