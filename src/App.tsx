import { useState } from 'react'
import './App.css'

interface Option {
  label: string
  url: string
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE'
}
const comment = 'メッセージに@Peepleとかを入れてもメンションができません。以下の<@&role_id>を代わりに使ってください。'
const peeple_role_comment = '@Peeple:　　<@&1122017976356434066>'
const leeple_role = '@Leeple:　　<@&1140645113467523087>'
const play_url = import.meta.env.VITE_PLAY_WEBHOOK_URL
const create_url = import.meta.env.VITE_CREATE_WEBHOOK_URL
const draft_url = import.meta.env.VITE_DRAFT_WEBHOOK_URL

function App() {
  
  const [loading, setLoading] = useState(false)
  const [response, setResponse] = useState<string>('')
  const [error, setError] = useState<string>('')
  const [message, setMessage] = useState<string>('')
  const [selectedOption, setSelectedOption] = useState<string>('0')

  const post_options: Option[] = [
    { label: 'プレイ会', url: play_url, method: 'POST' },
    { label: '制作会', url: create_url, method: 'POST' },
    { label: '運営用草稿チャンネル', url: draft_url, method: 'POST' },
  ]

  const handleRequest = async (option: Option, message: string) => {
    setLoading(true)
    setError('')
    setResponse('')

    try {
      const response = await fetch(option.url, {
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
      setResponse("送信に成功しました！")
    } catch (err) {
      console.error('Request failed:', play_url)
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
    </div>
  )
}

export default App
