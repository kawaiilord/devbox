import { useEffect, useRef } from 'react'
import type { ReactNode } from 'react'
import { X } from 'lucide-react'
export function Avatar({
  name,
  color = '#678574',
  size = 'medium',
}: {
  name: string
  color?: string
  size?: string
}) {
  return (
    <span className={'avatar ' + size} style={{ '--avatar': color } as React.CSSProperties}>
      <span className="avatar-shape" />
      <span className="avatar-letter">{name.slice(-1)}</span>
    </span>
  )
}

export function Modal({
  title,
  children,
  onClose,
  wide = false,
}: {
  title: string
  children: ReactNode
  onClose: () => void
  wide?: boolean
}) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      if (e.key === 'Tab') {
        const nodes = ref.current?.querySelectorAll<HTMLElement>(
          'button,input,textarea,select,a[href]',
        )
        if (!nodes?.length) return
        const first = nodes[0],
          last = nodes[nodes.length - 1]
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', handler)
    ref.current?.querySelector<HTMLElement>('input,textarea,button')?.focus()
    return () => {
      document.removeEventListener('keydown', handler)
      previous?.focus()
    }
  }, [onClose])
  return (
    <div
      className="modal-shade"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        ref={ref}
        className={'modal ' + (wide ? 'wide' : '')}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="modal-top">
          <h2>{title}</h2>
          <button className="icon-button" aria-label="关闭" onClick={onClose}>
            <X size={20} />
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}

export function WorldScene({ compact = false }: { compact?: boolean }) {
  return (
    <div className={'world-scene ' + (compact ? 'compact' : '')} aria-label="宇宙地点示意图">
      <svg viewBox="0 0 800 330" role="img" aria-label="有书店、咖啡馆和步道的小城">
        <defs>
          <pattern id="dots" width="25" height="25" patternUnits="userSpaceOnUse">
            <circle cx="1" cy="1" r="1" fill="#b7c7b7" opacity=".4" />
          </pattern>
        </defs>
        <rect width="800" height="330" fill="#eaf0df" />
        <rect width="800" height="330" fill="url(#dots)" />
        <path
          d="M-20 275C127 158 267 351 402 208S620 119 847 42"
          fill="none"
          stroke="#d0e3d9"
          strokeWidth="70"
        />
        <path
          d="M-20 275C127 158 267 351 402 208S620 119 847 42"
          fill="none"
          stroke="#b2d3c6"
          strokeWidth="30"
        />
        <path d="M78 91L652 270M142 302L581 28" fill="none" stroke="#faf8ee" strokeWidth="35" />
        <path
          d="M78 91L652 270M142 302L581 28"
          fill="none"
          stroke="#d9d2b9"
          strokeWidth="1"
          strokeDasharray="6 8"
        />
        <g transform="translate(164 76)">
          <rect x="0" y="33" width="106" height="75" rx="5" fill="#d8bd96" />
          <path d="M-9 37L54 0L115 37Z" fill="#879783" />
          <rect x="42" y="66" width="22" height="42" rx="10" fill="#7d8f7e" />
          <rect x="12" y="57" width="20" height="24" rx="3" fill="#fcf5df" />
          <rect x="75" y="57" width="20" height="24" rx="3" fill="#fcf5df" />
          <path d="M-5 110H112" stroke="#799081" strokeWidth="4" strokeLinecap="round" />
        </g>
        <g transform="translate(524 143)">
          <rect x="0" y="15" width="125" height="83" rx="6" fill="#e2cfb2" />
          <path d="M-5 15L130 15L113-8H10Z" fill="#c29677" />
          <rect x="13" y="36" width="53" height="38" rx="3" fill="#f8f4dd" />
          <rect x="83" y="44" width="25" height="54" rx="3" fill="#a69e81" />
          <path d="M5 31H121" stroke="#7c987e" strokeWidth="12" strokeDasharray="12 9" />
        </g>
        <g fill="#9bb798">
          <ellipse cx="372" cy="59" rx="24" ry="31" />
          <ellipse cx="707" cy="243" rx="26" ry="31" />
          <ellipse cx="106" cy="222" rx="20" ry="27" />
          <ellipse cx="467" cy="272" rx="20" ry="24" />
        </g>
        <g stroke="#7e947b" strokeWidth="4">
          <path d="M372 65v28M707 253v28M106 230v22M467 281v15" />
        </g>
        <g fill="#fff" stroke="#d9e0d0">
          <rect x="171" y="195" width="112" height="32" rx="16" />
          <rect x="570" y="82" width="120" height="32" rx="16" />
        </g>
        <g fontSize="12" fill="#5f745b" textAnchor="middle" fontFamily="sans-serif">
          <text x="227" y="216">
            银杏书店
          </text>
          <text x="630" y="103">
            海风咖啡馆
          </text>
        </g>
      </svg>
      <span className="map-tag">
        <span className="live-dot" />
        生活正在发生
      </span>
    </div>
  )
}
